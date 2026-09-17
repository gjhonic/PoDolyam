//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"podolyam/internal/auth"
	"podolyam/internal/httpapi"
	"podolyam/internal/meetings"
	"podolyam/internal/money"
	"podolyam/internal/storage"
	"podolyam/migrations"
)

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("Задайте TEST_DATABASE_URL или make test-integration")
	}
	ctx := context.Background()
	base, err := storage.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "pd_test_" + auth.Token()[:16]
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); _, _ = base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); base.Close() })
	return db
}
func TestMigrations(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	// Два мигратора сериализуются advisory lock и не применяют SQL дважды.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- storage.Migrate(ctx, db, migrations.Files) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	body, err := fs.ReadFile(migrations.Files, "001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	changed := fstest.MapFS{"001_initial.sql": &fstest.MapFile{Data: append(body, ' ')}}
	if err = storage.Migrate(ctx, db, changed); err == nil {
		t.Fatal("изменённый checksum принят")
	}
	failed := fstest.MapFS{"001_initial.sql": &fstest.MapFile{Data: body}, "002_broken.sql": &fstest.MapFile{Data: []byte("CREATE TABLE should_rollback(id integer); SELECT missing_column;")}}
	if err = storage.Migrate(ctx, db, failed); err == nil {
		t.Fatal("сломанный SQL принят")
	}
	var absent bool
	if err = db.QueryRow(ctx, "SELECT to_regclass('should_rollback') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("миграция не откатилась")
	}
}

type browser struct {
	t          *testing.T
	client     *http.Client
	base, csrf string
}

func newBrowser(t *testing.T, base string) *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, client: &http.Client{Jar: jar}, base: base}
}
func (b *browser) request(method, path string, input any, status int) []byte {
	b.t.Helper()
	var payload []byte
	if input != nil {
		var err error
		payload, err = json.Marshal(input)
		if err != nil {
			b.t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, b.base+path, bytes.NewReader(payload))
	if err != nil {
		b.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if method != "GET" {
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("X-CSRF-Token", b.csrf)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		b.t.Fatal(err)
	}
	if resp.StatusCode != status {
		b.t.Fatalf("%s %s: %d != %d: %s", method, path, resp.StatusCode, status, data)
	}
	return data
}
func (b *browser) register(email string) {
	data := b.request("GET", "/api/auth/session", nil, 200)
	var session struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		b.t.Fatal(err)
	}
	b.csrf = session.CSRF
	data = b.request("POST", "/api/auth/register", map[string]string{"email": email, "password": "надёжный-пароль-123"}, 201)
	if err := json.Unmarshal(data, &session); err != nil {
		b.t.Fatal(err)
	}
	b.csrf = session.CSRF
}
func readJSON[T any](t *testing.T, data []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func ptr(v int64) *int64 { return &v }
func TestAPIFlow(t *testing.T) {
	db := database(t)
	ctx := context.Background()
	if err := storage.Migrate(ctx, db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	handler, err := httpapi.NewApp(db, "http://localhost:5173")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	a, b := newBrowser(t, server.URL), newBrowser(t, server.URL)
	a.register("first@example.test")
	b.register("second@example.test")
	a.request("GET", "/readyz", nil, 200)
	d := meetings.Draft{Title: "Ужин", Date: "2026-09-16", Bill: money.Bill{
		Participants: []money.Participant{{ID: "p", Name: "Женя", Order: 0}, {ID: "g", Name: "Дима", Order: 1}},
		PayerID:      "p", ReceiptTotal: ptr(201),
		Items: []money.Item{
			{ID: "mine", Name: "Личное", Quantity: 1, UnitPrice: ptr(100), Assignment: &money.Assignment{Mode: money.Single, Weights: []money.Weight{{ParticipantID: "p", Value: 1}}}},
			{ID: "shared", Name: "Общее", Quantity: 1, UnitPrice: ptr(101), Assignment: &money.Assignment{Mode: money.Equal, Weights: []money.Weight{{ParticipantID: "p", Value: 1}, {ParticipantID: "g", Value: 1}}}},
		},
	}}
	m := readJSON[meetings.Meeting](t, a.request("POST", "/api/meetings", d, 201))
	path := "/api/meetings/" + m.ID
	// Чужой ID не даёт права читать или менять данные.
	b.request("GET", path, nil, 404)
	update := struct {
		meetings.Draft
		Version int64 `json:"version"`
	}{d, m.Version}
	b.request("PUT", path, update, 404)
	oldCSRF := a.csrf
	a.csrf = "wrong"
	a.request("PUT", path, update, 403)
	a.csrf = oldCSRF
	a.request("PUT", path, update, 204)
	a.request("PUT", path, update, 409)
	m = readJSON[meetings.Meeting](t, a.request("GET", path, nil, 200))
	a.request("POST", path+"/finalize", map[string]int64{"version": m.Version}, 204)
	a.request("PUT", path, update, 409)
	m = readJSON[meetings.Meeting](t, a.request("GET", path, nil, 200))
	if *m.Calculation.ToRepay != 50 {
		t.Fatal("долг рассчитан неправильно")
	}
	// Даже прямой SQL не может заменить снимок.
	if _, err = db.Exec(ctx, "UPDATE meetings SET snapshot='{}' WHERE id=$1", m.ID); err == nil {
		t.Fatal("снимок изменён")
	}
	link := readJSON[map[string]string](t, a.request("POST", path+"/links", map[string]any{"participant_id": "g"}, 200))
	guest := newBrowser(t, server.URL)
	raw := guest.request("GET", "/api/shared/"+link["token"], nil, 200)
	personal := readJSON[meetings.Personal](t, raw)
	if personal.Total != 50 || len(personal.Lines) != 1 || strings.Contains(string(raw), "Женя") || strings.Contains(string(raw), "Личное") {
		t.Fatal("персональная ссылка раскрывает чужие данные")
	}
	guest.request("POST", path+"/payments", map[string]any{"participant_id": "g", "amount": 1}, 401)
	a.request("POST", path+"/payments", map[string]any{"participant_id": "g", "amount": 20}, 204)
	a.request("POST", path+"/payments", map[string]any{"participant_id": "g", "amount": 31}, 422)
	a.request("POST", path+"/close", nil, 409)
	m = readJSON[meetings.Meeting](t, a.request("GET", path, nil, 200))
	a.request("POST", path+"/payments/"+m.Payments[0].ID+"/cancel", map[string]string{"reason": "Ошибочная запись"}, 204)
	personal = readJSON[meetings.Personal](t, guest.request("GET", "/api/shared/"+link["token"], nil, 200))
	if personal.Remaining != 50 {
		t.Fatal("отмена не вернула остаток")
	}
	// Конкурентные получения по 30 при долге 50: успешным может быть только одно.
	var owner string
	if err = db.QueryRow(ctx, "SELECT owner_id::text FROM meetings WHERE id=$1", m.ID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	store := &meetings.Store{DB: db}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- store.Pay(ctx, owner, m.ID, "g", 30) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("успешных одновременных переводов: %d", success)
	}
	a.request("POST", path+"/payments", map[string]any{"participant_id": "g", "amount": 20}, 204)
	a.request("POST", path+"/close", nil, 204)
	a.request("POST", path+"/payments", map[string]any{"participant_id": "g", "amount": 1}, 409)
	a.request("POST", path+"/links", map[string]any{"participant_id": "g", "revoke": true}, 200)
	guest.request("GET", "/api/shared/"+link["token"], nil, 404)
	a.request("POST", "/api/auth/logout", nil, 204)
	a.request("GET", path, nil, 401)
	// Вход обновляет cookie и CSRF; неверный пароль не раскрывает наличие email.
	data := a.request("GET", "/api/auth/session", nil, 200)
	a.csrf = readJSON[map[string]any](t, data)["csrf"].(string)
	a.request("POST", "/api/auth/login", map[string]string{"email": "first@example.test", "password": "неверный-пароль-123"}, 401)
	data = a.request("POST", "/api/auth/login", map[string]string{"email": "first@example.test", "password": "надёжный-пароль-123"}, 200)
	a.csrf = readJSON[map[string]any](t, data)["csrf"].(string)
	a.request("GET", path, nil, 200)
}
