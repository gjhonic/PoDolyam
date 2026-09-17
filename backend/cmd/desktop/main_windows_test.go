package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"podolyam/internal/local"
	"podolyam/internal/meetings"
)

func TestDesktopBridge(t *testing.T) {
	store, err := local.Open(filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &App{ctx: context.Background(), store: store}
	session, err := app.Request("GET", "/api/auth/session", "")
	if err != nil || !json.Valid([]byte(session)) {
		t.Fatalf("session: %s %v", session, err)
	}
	profileJSON, err := app.Request("PUT", "/api/profile", `{"name":"Женя","phone":"+7 999 123-45-67","bank":"Т-Банк"}`)
	if err != nil {
		t.Fatal(err)
	}
	var profile local.TransferProfile
	if err = json.Unmarshal([]byte(profileJSON), &profile); err != nil || profile.Bank != "Т-Банк" {
		t.Fatalf("profile: %s %v", profileJSON, err)
	}
	loadedProfile, err := app.Request("GET", "/api/profile", "")
	if err != nil || loadedProfile != profileJSON {
		t.Fatalf("load profile: %s %v", loadedProfile, err)
	}
	if _, err = app.Request("PUT", "/api/profile", `{"name":"Женя","phone":"","bank":"Т-Банк"}`); err == nil {
		t.Fatal("accepted profile without phone")
	}
	friendJSON, err := app.Request("POST", "/api/friends", `{"id":"","name":"Дима","phone":"+7 900","birthday":"1990-09-17"}`)
	if err != nil {
		t.Fatal(err)
	}
	var friend local.Friend
	if err = json.Unmarshal([]byte(friendJSON), &friend); err != nil || friend.ID == "" {
		t.Fatalf("friend: %s %v", friendJSON, err)
	}
	friendsJSON, err := app.Request("GET", "/api/friends", "")
	if err != nil || !strings.Contains(friendsJSON, friend.ID) {
		t.Fatalf("friends: %s %v", friendsJSON, err)
	}
	if _, err = app.Request("DELETE", "/api/friends/"+friend.ID, ""); err != nil {
		t.Fatal(err)
	}
	// Интерфейс передаёт JSON через Wails: проверяем границу без сетевого сервера.
	created, err := app.Request("POST", "/api/meetings", `{"title":"Ужин","date":"2026-09-17","venue":"","bill":{"participants":[],"payer_id":"","items":[],"receipt_total":null}}`)
	if err != nil {
		t.Fatal(err)
	}
	var meeting meetings.Meeting
	if err := json.Unmarshal([]byte(created), &meeting); err != nil {
		t.Fatal(err)
	}
	if meeting.Bill.PayerID != "me" || len(meeting.Bill.Participants) != 1 || meeting.Bill.Participants[0].Name != "Женя" {
		t.Fatalf("организатор не назначен плательщиком: %+v", meeting.Bill)
	}
	loaded, err := app.Request("GET", "/api/meetings/"+meeting.ID, "")
	if err != nil || !json.Valid([]byte(loaded)) {
		t.Fatalf("load: %s %v", loaded, err)
	}
	for _, body := range []string{`{"unknown":true}`, `{} {}`} {
		if _, err := app.Request("POST", "/api/meetings", body); err == nil {
			t.Fatalf("accepted invalid JSON: %s", body)
		}
	}
	if _, err := app.Request("POST", "/api/auth/register", "{}"); err == nil {
		t.Fatal("desktop must not register server accounts")
	}
	if _, err := app.Request("POST", "/api/meetings/"+meeting.ID+"/finalize", `{"version":1}`); err == nil {
		t.Fatal("empty draft finalized")
	}
}
