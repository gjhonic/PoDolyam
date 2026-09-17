package local

import (
	"context"
	"database/sql"
	"errors"

	_ "modernc.org/sqlite"
	"path/filepath"
	"sync"
	"testing"

	"podolyam/internal/demo"
	"podolyam/internal/meetings"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestLocalLifecycle(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	draft := meetings.Draft{Title: "Ужин", Description: "Встреча после релиза", Date: "2026-09-17", Bill: demo.Restaurant()}
	m, err := s.Create(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}
	if m.Description != draft.Description {
		t.Fatal("описание встречи не сохранено")
	}
	if err = s.Save(ctx, m.ID, 1, draft); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, m.ID, 1, draft); err == nil {
		t.Fatal("принята устаревшая версия")
	}
	if err = s.Finalize(ctx, m.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err = s.Save(ctx, m.ID, 3, draft); err == nil {
		t.Fatal("изменён зафиксированный счёт")
	}
	if _, err = s.db.Exec("UPDATE meetings SET payload=json_set(payload,'$.bill.receipt_total',1) WHERE id=?", m.ID); err == nil {
		t.Fatal("триггер разрешил изменение снимка")
	}
	if _, err = s.db.Exec("UPDATE meetings SET payload=json_set(payload,'$.description','Другое') WHERE id=?", m.ID); err == nil {
		t.Fatal("триггер разрешил изменение описания зафиксированной встречи")
	}
	personal, err := s.Personal(ctx, m.ID, "dima")
	if err != nil {
		t.Fatal(err)
	}
	if personal.Total != 152168 || personal.Remaining != 152168 {
		t.Fatalf("%+v", personal)
	}
	for _, line := range personal.Lines {
		if line.Name == "Токпоки Карбонара" {
			t.Fatal("раскрыта чужая доля")
		}
	}
	if err = s.Pay(ctx, m.ID, "dima", 100); err != nil {
		t.Fatal(err)
	}
	if err = s.Pay(ctx, m.ID, "dima", 152069); err == nil {
		t.Fatal("разрешена переплата")
	}
	m, err = s.Get(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(ctx, m.ID, m.Payments[0].ID, "Неверная запись"); err != nil {
		t.Fatal(err)
	}
	m, err = s.Get(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Payments) != 1 || m.Payments[0].CancelledAt == nil || m.Remaining["dima"] != 152168 {
		t.Fatal("не сохранена история отмены")
	}
	if err = s.Finish(ctx, m.ID); err == nil {
		t.Fatal("закрыт непогашенный долг")
	}
	for id, amount := range m.Remaining {
		if amount > 0 {
			if err = s.Pay(ctx, m.ID, id, amount); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = s.Finish(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Cancel(ctx, m.ID, m.Payments[0].ID, "Повтор"); err == nil {
		t.Fatal("закрытая встреча изменена")
	}
}
func TestPersistenceAndConcurrentPayments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m, err := s.Create(ctx, meetings.Draft{Title: "Сохранение", Date: "2026-09-17", Bill: demo.Restaurant()})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Finalize(ctx, m.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err = s.Get(ctx, m.ID)
	if err != nil || m.Calculation.Total != 882000 {
		t.Fatalf("перезапуск: %v", err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.Pay(ctx, m.ID, "dima", 100000) }()
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
		t.Fatalf("успешных платежей: %d", success)
	}
}

func TestTransferProfile(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	profile, err := s.GetProfile(ctx)
	if err != nil || profile != (TransferProfile{}) {
		t.Fatalf("пустой профиль: %+v %v", profile, err)
	}
	if _, err = s.SaveProfile(ctx, TransferProfile{Name: "Женя", Phone: " ", Bank: "Банк"}); err == nil {
		t.Fatal("сохранён профиль без телефона")
	}
	saved, err := s.SaveProfile(ctx, TransferProfile{Name: " Женя ", Phone: " +7 999 123-45-67 ", Bank: " Т-Банк "})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Женя" || saved.Phone != "+7 999 123-45-67" || saved.Bank != "Т-Банк" {
		t.Fatalf("значения не нормализованы: %+v", saved)
	}
	loaded, err := s.GetProfile(ctx)
	if err != nil || loaded != saved {
		t.Fatalf("профиль не сохранён: %+v %v", loaded, err)
	}
}

func TestMigratesVersionOneDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE meetings(id TEXT PRIMARY KEY,payload TEXT NOT NULL CHECK(json_valid(payload)),created_at TEXT NOT NULL); PRAGMA user_version=1;`)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveProfile(context.Background(), TransferProfile{Name: "Женя", Phone: "+7 900 000-00-00", Bank: "Сбер"}); err != nil {
		t.Fatal(err)
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatalf("версия схемы: %d, %v", version, err)
	}
}

func TestFriendsLifecycle(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if _, err := s.SaveFriend(ctx, Friend{Name: "Дима", Phone: "+7 900", Birthday: "17 сентября"}); err == nil {
		t.Fatal("принята некорректная дата")
	}
	friend, err := s.SaveFriend(ctx, Friend{Name: " Дима ", Phone: " +7 900 000-00-00 ", Birthday: "1990-09-17"})
	if err != nil {
		t.Fatal(err)
	}
	if friend.ID == "" || friend.Name != "Дима" {
		t.Fatalf("друг не нормализован: %+v", friend)
	}
	friend.Phone = "+7 901 111-11-11"
	if _, err = s.SaveFriend(ctx, friend); err != nil {
		t.Fatal(err)
	}
	friends, err := s.ListFriends(ctx)
	if err != nil || len(friends) != 1 || friends[0].Phone != friend.Phone {
		t.Fatalf("список друзей: %+v %v", friends, err)
	}
	if err = s.DeleteFriend(ctx, friend.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteFriend(ctx, friend.ID); !errors.Is(err, meetings.ErrNotFound) {
		t.Fatalf("повторное удаление: %v", err)
	}
}

func TestMigratesVersionTwoProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE transfer_profile(id INTEGER PRIMARY KEY CHECK(id=1),phone TEXT NOT NULL,bank TEXT NOT NULL); INSERT INTO transfer_profile(id,phone,bank) VALUES(1,'+7 900','Сбер'); PRAGMA user_version=2;`)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	profile, err := s.GetProfile(context.Background())
	if err != nil || profile.Phone != "+7 900" || profile.Bank != "Сбер" || profile.Name != "" {
		t.Fatalf("миграция профиля: %+v %v", profile, err)
	}
	if _, err = s.SaveProfile(context.Background(), TransferProfile{Name: "Женя", Phone: profile.Phone, Bank: profile.Bank}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMeeting(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	meeting, err := s.Create(ctx, meetings.Draft{Title: "Удалить", Date: "2026-09-17", Bill: demo.Restaurant()})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, meeting.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, meeting.ID); !errors.Is(err, meetings.ErrNotFound) {
		t.Fatalf("встреча осталась после удаления: %v", err)
	}
	if err = s.Delete(ctx, meeting.ID); !errors.Is(err, meetings.ErrNotFound) {
		t.Fatalf("повторное удаление: %v", err)
	}
}

func TestEnsureOrganizerRepairsLegacyDraft(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	bill := demo.Restaurant()
	bill.PayerID = ""
	meeting, err := s.Create(ctx, meetings.Draft{Title: "Старый черновик", Date: "2026-09-17", Bill: bill})
	if err != nil {
		t.Fatal(err)
	}
	var organizerID string
	for _, participant := range bill.Participants {
		if participant.Name == "Женя" {
			organizerID = participant.ID
		}
	}
	if organizerID == "" {
		t.Fatal("в демоданных нет организатора")
	}
	if err = s.EnsureOrganizer(ctx, meeting.ID, "Женя"); err != nil {
		t.Fatal(err)
	}
	repaired, err := s.Get(ctx, meeting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Bill.PayerID != organizerID || repaired.Version != 2 {
		t.Fatalf("организатор не восстановлен: %+v", repaired)
	}
	if err = s.EnsureOrganizer(ctx, meeting.ID, "Женя"); err != nil {
		t.Fatal(err)
	}
	again, err := s.Get(ctx, meeting.ID)
	if err != nil || again.Version != repaired.Version {
		t.Fatalf("повторное открытие изменило черновик: %+v %v", again, err)
	}
}
