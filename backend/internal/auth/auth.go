// Package auth управляет паролями и серверными сессиями, без привязки к HTTP.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

var ErrCredentials = errors.New("неверный email или пароль")
var ErrInvalid = errors.New("нужны email и пароль от 12 символов до 128 байт")
var ErrLimited = errors.New("слишком много попыток, повторите позже")
var ErrConflict = errors.New("не удалось зарегистрировать аккаунт")

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type Session struct {
	Token   string
	CSRF    string
	User    *User
	Expires time.Time
}
type Store struct {
	DB    *pgxpool.Pool
	slots chan struct{}
}

func New(db *pgxpool.Pool) *Store { return &Store{DB: db, slots: make(chan struct{}, 2)} }
func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Digest(value string) []byte { h := sha256.Sum256([]byte(value)); return h[:] }

// Ограничение хранится в БД, поэтому переживает перезапуск процесса.
// Ключи хешируются: email и IP не сохраняются в таблице лимитов открытым текстом.
func (s *Store) Allow(ctx context.Context, key string, limit int) error {
	var count int
	err := s.DB.QueryRow(ctx, `INSERT INTO rate_limits(key,count,expires_at) VALUES($1,1,now()+interval '15 minutes')
 ON CONFLICT(key) DO UPDATE SET
 count=CASE WHEN rate_limits.expires_at<=now() THEN 1 ELSE rate_limits.count+1 END,
 expires_at=CASE WHEN rate_limits.expires_at<=now() THEN now()+interval '15 minutes' ELSE rate_limits.expires_at END RETURNING count`, Digest(key)).Scan(&count)
	if err != nil {
		return err
	}
	if count > limit {
		return ErrLimited
	}
	return nil
}
func (s *Store) Get(ctx context.Context, token string) (Session, error) {
	var out Session
	var id, email *string
	err := s.DB.QueryRow(ctx, `SELECT s.csrf,s.expires_at,u.id::text,u.email FROM sessions s LEFT JOIN users u ON u.id=s.user_id WHERE token_hash=$1 AND expires_at>now()`, Digest(token)).Scan(&out.CSRF, &out.Expires, &id, &email)
	if err != nil {
		return out, err
	}
	out.Token = token
	if id != nil {
		out.User = &User{ID: *id, Email: *email}
	}
	return out, nil
}
func (s *Store) Anonymous(ctx context.Context) (Session, error) {
	out := Session{Token: Token(), CSRF: Token(), Expires: time.Now().Add(30 * time.Minute)}
	// Уборка выполняется при создании сессии; индексы ограничивают стоимость поиска.
	if _, err := s.DB.Exec(ctx, "DELETE FROM sessions WHERE expires_at<=now()"); err != nil {
		return Session{}, err
	}
	if _, err := s.DB.Exec(ctx, "DELETE FROM rate_limits WHERE expires_at<=now()"); err != nil {
		return Session{}, err
	}
	_, err := s.DB.Exec(ctx, "INSERT INTO sessions(token_hash,csrf,expires_at) VALUES($1,$2,$3)", Digest(out.Token), out.CSRF, out.Expires)
	return out, err
}
func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.DB.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", Digest(token))
	return err
}
func normalize(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
func (s *Store) Authenticate(ctx context.Context, old Session, email, password string, register bool) (Session, error) {
	email = normalize(email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 || !utf8.ValidString(password) || len(password) > 128 || utf8.RuneCountInString(password) < 12 {
		return Session{}, ErrInvalid
	}
	if err = s.Allow(ctx, "email:"+email, 10); err != nil {
		return Session{}, err
	}
	// Argon2 требует память. Два одновременных вычисления защищают процесс от
	// лавины запросов. Новых горутин здесь нет; занятые слоты дают HTTP 429.
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Session{}, ErrLimited
	}
	salt := make([]byte, 16)
	expected := make([]byte, 32)
	var user User
	if register {
		if _, err = rand.Read(salt); err != nil {
			return Session{}, err
		}
	} else {
		err = s.DB.QueryRow(ctx, "SELECT id::text,email,password_salt,password_hash FROM users WHERE email=$1", email).Scan(&user.ID, &user.Email, &salt, &expected)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Session{}, err
		}
	}
	// Даже для неизвестного email выполняем Argon2, не выдавая наличие аккаунта.
	hash := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	if !register && (user.ID == "" || subtle.ConstantTimeCompare(hash, expected) != 1) {
		return Session{}, ErrCredentials
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	// Старая сессия блокируется: повтор запроса входа не создаст вторую сессию.
	var marker int
	if err = tx.QueryRow(ctx, "SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at>now() FOR UPDATE", Digest(old.Token)).Scan(&marker); err != nil {
		return Session{}, err
	}
	if register {
		err = tx.QueryRow(ctx, "INSERT INTO users(email,password_salt,password_hash) VALUES($1,$2,$3) RETURNING id::text,email", email, salt, hash).Scan(&user.ID, &user.Email)
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			return Session{}, ErrConflict
		}
		if err != nil {
			return Session{}, err
		}
	}
	out := Session{Token: Token(), CSRF: Token(), User: &user, Expires: time.Now().Add(7 * 24 * time.Hour)}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", Digest(old.Token)); err != nil {
		return Session{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,csrf,expires_at) VALUES($1,$2,$3,$4)", Digest(out.Token), user.ID, out.CSRF, out.Expires); err != nil {
		return Session{}, err
	}
	return out, tx.Commit(ctx)
}
