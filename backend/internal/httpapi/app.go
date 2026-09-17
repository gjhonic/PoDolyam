package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"podolyam/internal/auth"
	"podolyam/internal/meetings"
	"podolyam/internal/money"
)

type app struct {
	db             *pgxpool.Pool
	auth           *auth.Store
	meetings       *meetings.Store
	origin, cookie string
	secure         bool
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var shareToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

func NewApp(db *pgxpool.Pool, origin string) (http.Handler, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("APP_ORIGIN должен быть точным origin без пути")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return nil, errors.New("для внешнего домена требуется HTTPS")
	}
	a := &app{db: db, auth: auth.New(db), meetings: &meetings.Store{DB: db}, origin: origin, cookie: "podolyam_session", secure: u.Scheme == "https"}
	if a.secure {
		a.cookie = "__Host-podolyam_session"
	}
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", NewHandler())
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			fail(w, 503, "unavailable", "База данных недоступна")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/auth/session", a.session)
	mux.HandleFunc("POST /api/auth/register", a.signIn(true))
	mux.HandleFunc("POST /api/auth/login", a.signIn(false))
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.guard(w, r, true)
		if !ok {
			return
		}
		if err := a.auth.Logout(r.Context(), s.Token); err != nil {
			problem(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: a.cookie, Value: "", Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/meetings", func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.guard(w, r, true)
		if !ok {
			return
		}
		out, err := a.meetings.List(r.Context(), s.User.ID)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, out)
	})
	mux.HandleFunc("POST /api/meetings", func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.guard(w, r, true)
		if !ok {
			return
		}
		var d meetings.Draft
		if !decode(w, r, &d) {
			return
		}
		out, err := a.meetings.Create(r.Context(), s.User.ID, d)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 201, out)
	})
	mux.HandleFunc("GET /api/meetings/{id}", a.meetingGet)
	mux.HandleFunc("PUT /api/meetings/{id}", a.meetingSave)
	mux.HandleFunc("POST /api/meetings/{id}/finalize", a.finalize)
	mux.HandleFunc("POST /api/meetings/{id}/payments", a.pay)
	mux.HandleFunc("POST /api/meetings/{id}/payments/{payment}/cancel", a.cancelPayment)
	mux.HandleFunc("POST /api/meetings/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.meetingGuard(w, r)
		if !ok {
			return
		}
		if err := a.meetings.Close(r.Context(), s.User.ID, r.PathValue("id")); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/meetings/{id}/links", a.link)
	mux.HandleFunc("GET /api/shared/{token}", func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		if !shareToken.MatchString(token) {
			fail(w, 404, "not_found", "Ссылка недоступна")
			return
		}
		out, err := a.meetings.Personal(r.Context(), token)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, out)
	})
	// Deadline проходит через HTTP → сервис → pgx. Отмена запроса прерывает SQL.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func problem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, meetings.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		fail(w, 404, "not_found", "Объект не найден или недоступен")
	case errors.Is(err, auth.ErrCredentials):
		fail(w, 401, "credentials", "Неверный email или пароль")
	case errors.Is(err, auth.ErrLimited):
		w.Header().Set("Retry-After", "900")
		fail(w, 429, "rate_limited", err.Error())
	case errors.Is(err, auth.ErrConflict), errors.Is(err, meetings.ErrConflict):
		fail(w, 409, "conflict", err.Error())
	case errors.Is(err, money.ErrInvalid), errors.Is(err, money.ErrIncomplete), errors.Is(err, meetings.ErrInvalid), errors.Is(err, auth.ErrInvalid):
		fail(w, 422, "validation", err.Error())
	default:
		slog.Error("Ошибка обработки запроса", "error", err)
		fail(w, 500, "internal", "Не удалось выполнить запрос")
	}
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		fail(w, 415, "content_type", "Нужен Content-Type: application/json")
		return false
	}
	// Один JSON-документ, ограниченный размер и отказ от неизвестных полей.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		fail(w, 400, "json", "Проверьте формат и размер JSON")
		return false
	}
	if err = d.Decode(new(any)); !errors.Is(err, io.EOF) {
		fail(w, 400, "json", "Ожидается один JSON-документ")
		return false
	}
	return true
}
func (a *app) cookieSession(w http.ResponseWriter, s auth.Session) {
	http.SetCookie(w, &http.Cookie{Name: a.cookie, Value: s.Token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, Expires: s.Expires, MaxAge: int(time.Until(s.Expires).Seconds())})
}
func sessionResponse(s auth.Session) any {
	return struct {
		CSRF string     `json:"csrf"`
		User *auth.User `json:"user"`
	}{s.CSRF, s.User}
}
func (a *app) get(r *http.Request) (auth.Session, error) {
	c, err := r.Cookie(a.cookie)
	if err != nil {
		return auth.Session{}, pgx.ErrNoRows
	}
	return a.auth.Get(r.Context(), c.Value)
}
func peer(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
func (a *app) session(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != a.origin) {
		fail(w, 403, "origin", "Недопустимый источник запроса")
		return
	}
	s, err := a.get(r)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = a.auth.Allow(r.Context(), "session:"+peer(r), 60); err != nil {
			problem(w, err)
			return
		}
		s, err = a.auth.Anonymous(r.Context())
		if err == nil {
			a.cookieSession(w, s)
		}
	}
	if err != nil {
		problem(w, err)
		return
	}
	respond(w, 200, sessionResponse(s))
}
func (a *app) guard(w http.ResponseWriter, r *http.Request, requireUser bool) (auth.Session, bool) {
	s, err := a.get(r)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "session", "Сессия истекла. Войдите снова")
		return s, false
	}
	if err != nil {
		problem(w, err)
		return s, false
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if r.Header.Get("Origin") != a.origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) != 1 {
			fail(w, 403, "csrf", "Обновите страницу и повторите действие")
			return s, false
		}
	}
	if requireUser && s.User == nil {
		fail(w, 401, "session", "Войдите в аккаунт")
		return s, false
	}
	return s, true
}
func (a *app) signIn(register bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.guard(w, r, false)
		if !ok {
			return
		}
		if err := a.auth.Allow(r.Context(), "auth:"+peer(r), 60); err != nil {
			problem(w, err)
			return
		}
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &input) {
			return
		}
		out, err := a.auth.Authenticate(r.Context(), s, input.Email, input.Password, register)
		if err != nil {
			problem(w, err)
			return
		}
		a.cookieSession(w, out)
		status := 200
		if register {
			status = 201
		}
		respond(w, status, sessionResponse(out))
	}
}
func (a *app) meetingGuard(w http.ResponseWriter, r *http.Request) (auth.Session, bool) {
	s, ok := a.guard(w, r, true)
	if !ok {
		return s, false
	}
	if !uuid.MatchString(r.PathValue("id")) {
		fail(w, 404, "not_found", "Встреча не найдена")
		return s, false
	}
	return s, true
}
func (a *app) meetingGet(w http.ResponseWriter, r *http.Request) {
	s, ok := a.meetingGuard(w, r)
	if !ok {
		return
	}
	out, err := a.meetings.Get(r.Context(), s.User.ID, r.PathValue("id"))
	if err != nil {
		problem(w, err)
		return
	}
	respond(w, 200, out)
}
func (a *app) meetingSave(w http.ResponseWriter, r *http.Request) {
	s, ok := a.meetingGuard(w, r)
	if !ok {
		return
	}
	var input struct {
		meetings.Draft
		Version int64 `json:"version"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := a.meetings.Save(r.Context(), s.User.ID, r.PathValue("id"), input.Version, input.Draft); err != nil {
		problem(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *app) finalize(w http.ResponseWriter, r *http.Request) {
	s, ok := a.meetingGuard(w, r)
	if !ok {
		return
	}
	var input struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := a.meetings.Finalize(r.Context(), s.User.ID, r.PathValue("id"), input.Version); err != nil {
		problem(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *app) pay(w http.ResponseWriter, r *http.Request) {
	s, ok := a.meetingGuard(w, r)
	if !ok {
		return
	}
	var input struct {
		Participant string `json:"participant_id"`
		Amount      int64  `json:"amount"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := a.meetings.Pay(r.Context(), s.User.ID, r.PathValue("id"), input.Participant, input.Amount); err != nil {
		problem(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *app) cancelPayment(w http.ResponseWriter, r *http.Request) {
	s, ok := a.meetingGuard(w, r)
	if !ok {
		return
	}
	if !uuid.MatchString(r.PathValue("payment")) {
		fail(w, 404, "not_found", "Перевод не найден")
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := a.meetings.CancelPayment(r.Context(), s.User.ID, r.PathValue("id"), r.PathValue("payment"), input.Reason); err != nil {
		problem(w, err)
		return
	}
	w.WriteHeader(204)
}
func (a *app) link(w http.ResponseWriter, r *http.Request) {
	s, ok := a.meetingGuard(w, r)
	if !ok {
		return
	}
	var input struct {
		Participant string `json:"participant_id"`
		Revoke      bool   `json:"revoke"`
	}
	if !decode(w, r, &input) {
		return
	}
	token, err := a.meetings.Link(r.Context(), s.User.ID, r.PathValue("id"), input.Participant, input.Revoke)
	if err != nil {
		problem(w, err)
		return
	}
	respond(w, 200, map[string]string{"token": token})
}
