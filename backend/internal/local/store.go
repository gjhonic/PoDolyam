// Package local — однопользовательское хранилище Windows-приложения.
// SQLite работает внутри процесса, отдельный сервер БД не нужен.
package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
	"podolyam/internal/meetings"
	"podolyam/internal/money"
)

type Store struct{ db *sql.DB }

type TransferProfile struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Bank  string `json:"bank"`
}

type Friend struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Birthday string `json:"birthday"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // Один локальный писатель: изменения выполняются последовательно.
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fail(err)
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 3 {
		return fail(errors.New("база создана более новой версией PoDolyam"))
	}
	if version == 0 {
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		defer tx.Rollback()
		_, err = tx.Exec(`CREATE TABLE meetings(id TEXT PRIMARY KEY,payload TEXT NOT NULL CHECK(json_valid(payload)),created_at TEXT NOT NULL);
  CREATE TRIGGER frozen_snapshot BEFORE UPDATE ON meetings
  WHEN json_extract(OLD.payload,'$.state')<>'draft' AND (
    json_extract(NEW.payload,'$.bill') IS NOT json_extract(OLD.payload,'$.bill') OR
    json_extract(NEW.payload,'$.calculation') IS NOT json_extract(OLD.payload,'$.calculation') OR
    json_extract(NEW.payload,'$.title') IS NOT json_extract(OLD.payload,'$.title') OR
    json_extract(NEW.payload,'$.description') IS NOT json_extract(OLD.payload,'$.description') OR
    json_extract(NEW.payload,'$.date') IS NOT json_extract(OLD.payload,'$.date') OR
    json_extract(NEW.payload,'$.venue') IS NOT json_extract(OLD.payload,'$.venue') OR
    json_extract(NEW.payload,'$.state')='draft')
  BEGIN SELECT RAISE(ABORT,'Зафиксированный расчёт неизменяем'); END;
  CREATE TABLE transfer_profile(id INTEGER PRIMARY KEY CHECK(id=1),name TEXT NOT NULL,phone TEXT NOT NULL,bank TEXT NOT NULL);
  CREATE TABLE friends(id TEXT PRIMARY KEY,name TEXT NOT NULL,phone TEXT NOT NULL,birthday TEXT NOT NULL,created_at TEXT NOT NULL);
  PRAGMA user_version=3;`)
		if err != nil {
			return fail(err)
		}
		if err = tx.Commit(); err != nil {
			return fail(err)
		}
	}
	if version == 1 {
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		defer tx.Rollback()
		_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS transfer_profile(id INTEGER PRIMARY KEY CHECK(id=1),name TEXT NOT NULL DEFAULT '',phone TEXT NOT NULL,bank TEXT NOT NULL);
  CREATE TABLE friends(id TEXT PRIMARY KEY,name TEXT NOT NULL,phone TEXT NOT NULL,birthday TEXT NOT NULL,created_at TEXT NOT NULL);
  PRAGMA user_version=3;`)
		if err != nil {
			return fail(err)
		}
		if err = tx.Commit(); err != nil {
			return fail(err)
		}
	}
	if version == 2 {
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		defer tx.Rollback()
		_, err = tx.Exec(`ALTER TABLE transfer_profile ADD COLUMN name TEXT NOT NULL DEFAULT '';
  CREATE TABLE friends(id TEXT PRIMARY KEY,name TEXT NOT NULL,phone TEXT NOT NULL,birthday TEXT NOT NULL,created_at TEXT NOT NULL);
  PRAGMA user_version=3;`)
		if err != nil {
			return fail(err)
		}
		if err = tx.Commit(); err != nil {
			return fail(err)
		}
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

func validateProfile(profile TransferProfile) (TransferProfile, error) {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Phone = strings.TrimSpace(profile.Phone)
	profile.Bank = strings.TrimSpace(profile.Bank)
	if profile.Name == "" || profile.Phone == "" || profile.Bank == "" || len([]rune(profile.Name)) > 120 || len([]rune(profile.Phone)) > 40 || len([]rune(profile.Bank)) > 120 {
		return TransferProfile{}, meetings.ErrInvalid
	}
	return profile, nil
}

func (s *Store) GetProfile(ctx context.Context) (TransferProfile, error) {
	var profile TransferProfile
	err := s.db.QueryRowContext(ctx, "SELECT name,phone,bank FROM transfer_profile WHERE id=1").Scan(&profile.Name, &profile.Phone, &profile.Bank)
	if errors.Is(err, sql.ErrNoRows) {
		return TransferProfile{}, nil
	}
	return profile, err
}

func (s *Store) SaveProfile(ctx context.Context, profile TransferProfile) (TransferProfile, error) {
	profile, err := validateProfile(profile)
	if err != nil {
		return TransferProfile{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO transfer_profile(id,name,phone,bank) VALUES(1,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,phone=excluded.phone,bank=excluded.bank`, profile.Name, profile.Phone, profile.Bank)
	return profile, err
}

func validateFriend(friend Friend) (Friend, error) {
	friend.Name = strings.TrimSpace(friend.Name)
	friend.Phone = strings.TrimSpace(friend.Phone)
	friend.Birthday = strings.TrimSpace(friend.Birthday)
	if friend.Name == "" || friend.Phone == "" || len([]rune(friend.Name)) > 120 || len([]rune(friend.Phone)) > 40 {
		return Friend{}, meetings.ErrInvalid
	}
	if friend.Birthday != "" {
		if _, err := time.Parse("2006-01-02", friend.Birthday); err != nil {
			return Friend{}, meetings.ErrInvalid
		}
	}
	return friend, nil
}

func (s *Store) ListFriends(ctx context.Context) ([]Friend, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,phone,birthday FROM friends ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	friends := []Friend{}
	for rows.Next() {
		var friend Friend
		if err = rows.Scan(&friend.ID, &friend.Name, &friend.Phone, &friend.Birthday); err != nil {
			return nil, err
		}
		friends = append(friends, friend)
	}
	return friends, rows.Err()
}

func (s *Store) SaveFriend(ctx context.Context, friend Friend) (Friend, error) {
	friend, err := validateFriend(friend)
	if err != nil {
		return Friend{}, err
	}
	if friend.ID == "" {
		friend.ID = uuid.NewString()
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO friends(id,name,phone,birthday,created_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,phone=excluded.phone,birthday=excluded.birthday`, friend.ID, friend.Name, friend.Phone, friend.Birthday, time.Now().UTC().Format(time.RFC3339Nano))
	return friend, err
}

func (s *Store) DeleteFriend(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM friends WHERE id=?", id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return meetings.ErrNotFound
	}
	return nil
}
func balances(m *meetings.Meeting) error {
	m.Remaining = map[string]int64{}
	if m.State == "draft" {
		var err error
		m.Calculation, err = money.Calculate(m.Bill)
		return err
	}
	for _, total := range m.Calculation.Totals {
		receipts := []int64{}
		for _, p := range m.Payments {
			if p.ParticipantID == total.ParticipantID && p.CancelledAt == nil {
				receipts = append(receipts, p.Amount)
			}
		}
		if total.Debt == nil {
			return errors.New("повреждён снимок: нет обязательства")
		}
		left, err := money.Remaining(*total.Debt, receipts)
		if err != nil {
			return err
		}
		m.Remaining[total.ParticipantID] = left
	}
	return nil
}
func decode(data string) (meetings.Meeting, error) {
	var m meetings.Meeting
	err := json.Unmarshal([]byte(data), &m)
	if err == nil {
		err = balances(&m)
	}
	return m, err
}
func (s *Store) Get(ctx context.Context, id string) (meetings.Meeting, error) {
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT payload FROM meetings WHERE id=?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return meetings.Meeting{}, meetings.ErrNotFound
	}
	if err != nil {
		return meetings.Meeting{}, err
	}
	return decode(data)
}
func (s *Store) List(ctx context.Context) ([]meetings.Summary, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM meetings ORDER BY created_at DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []meetings.Summary{}
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		m, err := decode(data)
		if err != nil {
			return nil, err
		}
		result = append(result, meetings.Summary{ID: m.ID, Title: m.Title, Date: m.Date, State: m.State})
	}
	return result, rows.Err()
}

// EnsureOrganizer восстанавливает плательщика только у старого черновика,
// если сохранённый payer_id пуст или больше не указывает на участника.
func (s *Store) EnsureOrganizer(ctx context.Context, id, organizerName string) error {
	organizerName = strings.TrimSpace(organizerName)
	if organizerName == "" {
		return meetings.ErrInvalid
	}
	current, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.State != "draft" {
		return nil
	}
	for _, participant := range current.Bill.Participants {
		if participant.ID == current.Bill.PayerID && current.Bill.PayerID != "" {
			return nil
		}
	}
	return s.change(ctx, id, func(meeting *meetings.Meeting) error {
		if meeting.State != "draft" {
			return nil
		}
		for _, participant := range meeting.Bill.Participants {
			if participant.ID == meeting.Bill.PayerID && meeting.Bill.PayerID != "" {
				return nil
			}
		}
		organizerID := ""
		maxOrder := int64(-1)
		for index := range meeting.Bill.Participants {
			participant := &meeting.Bill.Participants[index]
			if participant.Order > maxOrder {
				maxOrder = participant.Order
			}
			if participant.ID == "me" {
				participant.Name = organizerName
				organizerID = participant.ID
			}
		}
		if organizerID == "" {
			for _, participant := range meeting.Bill.Participants {
				if strings.EqualFold(strings.TrimSpace(participant.Name), organizerName) {
					organizerID = participant.ID
					break
				}
			}
		}
		if organizerID == "" {
			organizerID = "me"
			meeting.Bill.Participants = append(meeting.Bill.Participants, money.Participant{ID: organizerID, Name: organizerName, Order: maxOrder + 1})
		}
		meeting.Bill.PayerID = organizerID
		meeting.Version++
		return nil
	})
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM meetings WHERE id=?", id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return meetings.ErrNotFound
	}
	return nil
}

func (s *Store) Create(ctx context.Context, d meetings.Draft) (meetings.Meeting, error) {
	if err := meetings.ValidateDraft(d); err != nil {
		return meetings.Meeting{}, err
	}
	m := meetings.Meeting{ID: uuid.NewString(), Version: 1, State: "draft", Currency: "RUB", Draft: d, Payments: []meetings.Payment{}}
	if err := balances(&m); err != nil {
		return m, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO meetings(id,payload,created_at) VALUES(?,?,?)", m.ID, string(data), time.Now().UTC().Format(time.RFC3339Nano))
	return m, err
}

// Копия документа живёт только внутри транзакции. Ошибка callback не меняет БД.
func (s *Store) change(ctx context.Context, id string, fn func(*meetings.Meeting) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var data string
	err = tx.QueryRowContext(ctx, "SELECT payload FROM meetings WHERE id=?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return meetings.ErrNotFound
	}
	if err != nil {
		return err
	}
	m, err := decode(data)
	if err != nil {
		return err
	}
	if err = fn(&m); err != nil {
		return err
	}
	if err = balances(&m); err != nil {
		return err
	}
	next, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE meetings SET payload=? WHERE id=?", string(next), id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Save(ctx context.Context, id string, version int64, d meetings.Draft) error {
	if err := meetings.ValidateDraft(d); err != nil {
		return err
	}
	return s.change(ctx, id, func(m *meetings.Meeting) error {
		if m.State != "draft" || m.Version != version {
			return meetings.ErrConflict
		}
		next := map[string]money.Participant{}
		for _, p := range d.Bill.Participants {
			next[p.ID] = p
		}
		for _, p := range m.Bill.Participants {
			n, exists := next[p.ID]
			if exists && n.Order != p.Order {
				return fmt.Errorf("порядок участника нельзя менять")
			}
			if !exists {
				for _, item := range m.Bill.Items {
					if item.Assignment != nil {
						for _, w := range item.Assignment.Weights {
							if w.ParticipantID == p.ID {
								return fmt.Errorf("сначала снимите назначения участника и сохраните чек")
							}
						}
					}
				}
			}
		}
		m.Draft = d
		m.Version++
		return nil
	})
}
func (s *Store) Finalize(ctx context.Context, id string, version int64) error {
	return s.change(ctx, id, func(m *meetings.Meeting) error {
		if m.State != "draft" || m.Version != version {
			return meetings.ErrConflict
		}
		r, err := money.CalculateFinal(m.Bill)
		if err != nil {
			return err
		}
		m.Calculation = r
		m.State = "finalized"
		m.Version++
		return nil
	})
}
func (s *Store) Pay(ctx context.Context, id, participant string, amount int64) error {
	return s.change(ctx, id, func(m *meetings.Meeting) error {
		if m.State != "finalized" {
			return meetings.ErrConflict
		}
		left, ok := m.Remaining[participant]
		if !ok {
			return meetings.ErrNotFound
		}
		if _, err := money.Remaining(left, []int64{amount}); err != nil {
			return err
		}
		m.Payments = append(m.Payments, meetings.Payment{ID: uuid.NewString(), ParticipantID: participant, Amount: amount, CreatedAt: time.Now().UTC()})
		return nil
	})
}
func (s *Store) Cancel(ctx context.Context, id, payment, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return meetings.ErrInvalid
	}
	return s.change(ctx, id, func(m *meetings.Meeting) error {
		if m.State != "finalized" {
			return meetings.ErrConflict
		}
		for i := range m.Payments {
			p := &m.Payments[i]
			if p.ID == payment && p.CancelledAt == nil {
				now := time.Now().UTC()
				p.CancelledAt = &now
				p.Reason = &reason
				return nil
			}
		}
		return meetings.ErrNotFound
	})
}
func (s *Store) Finish(ctx context.Context, id string) error {
	return s.change(ctx, id, func(m *meetings.Meeting) error {
		if m.State != "finalized" {
			return meetings.ErrConflict
		}
		for _, amount := range m.Remaining {
			if amount != 0 {
				return errors.New("остались невозвращённые суммы")
			}
		}
		m.State = "closed"
		m.Version++
		return nil
	})
}
