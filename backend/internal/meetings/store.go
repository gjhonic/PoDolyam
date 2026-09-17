// Package meetings хранит встречи и выполняет изменения как единые транзакции.
package meetings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"podolyam/internal/auth"
	"podolyam/internal/money"
)

var ErrNotFound = errors.New("встреча не найдена")
var ErrConflict = errors.New("встреча уже изменена или зафиксирована")
var ErrInvalid = errors.New("некорректные данные встречи")

type Draft struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Date        string     `json:"date"`
	Venue       string     `json:"venue"`
	Bill        money.Bill `json:"bill"`
}
type Payment struct {
	ID            string     `json:"id"`
	ParticipantID string     `json:"participant_id"`
	Amount        int64      `json:"amount"`
	CreatedAt     time.Time  `json:"created_at"`
	CancelledAt   *time.Time `json:"cancelled_at"`
	Reason        *string    `json:"cancellation_reason"`
}
type Meeting struct {
	Remaining map[string]int64 `json:"remaining"`
	ID        string           `json:"id"`
	Version   int64            `json:"version"`
	State     string           `json:"state"`
	Currency  string           `json:"currency"`
	Draft
	Calculation money.Result `json:"calculation"`
	Payments    []Payment    `json:"payments"`
}
type Summary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date"`
	State string `json:"state"`
}
type Store struct{ DB *pgxpool.Pool }

const fields = "id::text,title,description,meeting_date::text,venue,state,version,bill,snapshot"

func read(row pgx.Row) (Meeting, error) {
	var m Meeting
	var bill, snapshot []byte
	err := row.Scan(&m.ID, &m.Title, &m.Description, &m.Date, &m.Venue, &m.State, &m.Version, &bill, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	m.Currency = "RUB"
	m.Remaining = map[string]int64{}
	m.Payments = []Payment{}
	if err = json.Unmarshal(bill, &m.Bill); err != nil {
		return m, err
	}
	if snapshot != nil {
		err = json.Unmarshal(snapshot, &m.Calculation)
	} else {
		m.Calculation, err = money.Calculate(m.Bill)
	}
	return m, err
}
func ValidateDraft(d Draft) error {
	if strings.TrimSpace(d.Title) == "" || utf8.RuneCountInString(d.Title) > 120 || utf8.RuneCountInString(d.Description) > 1000 || utf8.RuneCountInString(d.Venue) > 200 {
		return ErrInvalid
	}
	date, err := time.Parse(time.DateOnly, d.Date)
	if err != nil || date.Year() < 1900 || date.Year() > 9999 {
		return ErrInvalid
	}
	for _, p := range d.Bill.Participants {
		if strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 80 {
			return ErrInvalid
		}
	}
	for _, i := range d.Bill.Items {
		if strings.TrimSpace(i.Name) == "" || utf8.RuneCountInString(i.Name) > 160 {
			return ErrInvalid
		}
	}
	_, err = money.Calculate(d.Bill)
	return err
}
func (s *Store) Create(ctx context.Context, owner string, d Draft) (Meeting, error) {
	if err := ValidateDraft(d); err != nil {
		return Meeting{}, err
	}
	data, err := json.Marshal(d.Bill)
	if err != nil {
		return Meeting{}, err
	}
	return read(s.DB.QueryRow(ctx, "INSERT INTO meetings(owner_id,title,description,meeting_date,venue,bill) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+fields, owner, d.Title, d.Description, d.Date, d.Venue, data))
}
func (s *Store) List(ctx context.Context, owner string) ([]Summary, error) {
	rows, err := s.DB.Query(ctx, "SELECT id::text,title,meeting_date::text,state FROM meetings WHERE owner_id=$1 ORDER BY created_at DESC,id LIMIT 100", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var m Summary
		if err = rows.Scan(&m.ID, &m.Title, &m.Date, &m.State); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func loadPayments(ctx context.Context, tx pgx.Tx, m *Meeting) error {
	rows, err := tx.Query(ctx, "SELECT id::text,participant_id,amount,created_at,cancelled_at,cancellation_reason FROM payments WHERE meeting_id=$1 ORDER BY created_at,id", m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p Payment
		if err = rows.Scan(&p.ID, &p.ParticipantID, &p.Amount, &p.CreatedAt, &p.CancelledAt, &p.Reason); err != nil {
			return err
		}
		m.Payments = append(m.Payments, p)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if m.State != "draft" {
		for _, total := range m.Calculation.Totals {
			left, err := remaining(m, total.ParticipantID)
			if err != nil {
				return err
			}
			m.Remaining[total.ParticipantID] = left
		}
	}
	return nil
}
func (s *Store) Get(ctx context.Context, owner, id string) (Meeting, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Meeting{}, err
	}
	defer tx.Rollback(ctx)
	m, err := read(tx.QueryRow(ctx, "SELECT "+fields+" FROM meetings WHERE id=$1 AND owner_id=$2", id, owner))
	if err != nil {
		return m, err
	}
	if err = loadPayments(ctx, tx, &m); err != nil {
		return m, err
	}
	return m, tx.Commit(ctx)
}

// Любое изменение сначала блокирует строку встречи и проверяет владельца.
// Поэтому два одновременных перевода не могут оба потратить один остаток.
func (s *Store) change(ctx context.Context, owner, id string, fn func(pgx.Tx, *Meeting) error) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	m, err := read(tx.QueryRow(ctx, "SELECT "+fields+" FROM meetings WHERE id=$1 AND owner_id=$2 FOR UPDATE", id, owner))
	if err != nil {
		return err
	}
	if err = loadPayments(ctx, tx, &m); err != nil {
		return err
	}
	if err = fn(tx, &m); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Save(ctx context.Context, owner, id string, version int64, d Draft) error {
	if err := ValidateDraft(d); err != nil {
		return err
	}
	return s.change(ctx, owner, id, func(tx pgx.Tx, m *Meeting) error {
		if m.State != "draft" || m.Version != version {
			return ErrConflict
		}
		next := map[string]money.Participant{}
		for _, p := range d.Bill.Participants {
			next[p.ID] = p
		}
		for _, p := range m.Bill.Participants {
			n, exists := next[p.ID]
			if exists && n.Order != p.Order {
				return fmt.Errorf("%w: порядок участника неизменяем", ErrInvalid)
			}
			if !exists {
				for _, item := range m.Bill.Items {
					if item.Assignment != nil {
						for _, w := range item.Assignment.Weights {
							if w.ParticipantID == p.ID {
								return fmt.Errorf("%w: сначала снимите назначения участника и сохраните чек", ErrInvalid)
							}
						}
					}
				}
			}
		}
		data, err := json.Marshal(d.Bill)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE meetings SET title=$2,description=$3,meeting_date=$4,venue=$5,bill=$6,version=version+1 WHERE id=$1", id, d.Title, d.Description, d.Date, d.Venue, data)
		return err
	})
}
func (s *Store) Finalize(ctx context.Context, owner, id string, version int64) error {
	return s.change(ctx, owner, id, func(tx pgx.Tx, m *Meeting) error {
		if m.State != "draft" || m.Version != version {
			return ErrConflict
		}
		result, err := money.CalculateFinal(m.Bill)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE meetings SET snapshot=$2,state='finalized',version=version+1 WHERE id=$1", id, snapshot)
		return err
	})
}
func remaining(m *Meeting, participant string) (int64, error) {
	for _, total := range m.Calculation.Totals {
		if total.ParticipantID == participant && total.Debt != nil {
			receipts := []int64{}
			for _, p := range m.Payments {
				if p.ParticipantID == participant && p.CancelledAt == nil {
					receipts = append(receipts, p.Amount)
				}
			}
			return money.Remaining(*total.Debt, receipts)
		}
	}
	return 0, ErrNotFound
}
func (s *Store) Pay(ctx context.Context, owner, id, participant string, amount int64) error {
	return s.change(ctx, owner, id, func(tx pgx.Tx, m *Meeting) error {
		if m.State != "finalized" {
			return ErrConflict
		}
		left, err := remaining(m, participant)
		if err != nil {
			return err
		}
		if _, err = money.Remaining(left, []int64{amount}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO payments(meeting_id,participant_id,amount) VALUES($1,$2,$3)", id, participant, amount)
		return err
	})
}
func (s *Store) CancelPayment(ctx context.Context, owner, id, payment, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		return ErrInvalid
	}
	return s.change(ctx, owner, id, func(tx pgx.Tx, m *Meeting) error {
		if m.State != "finalized" {
			return ErrConflict
		}
		tag, err := tx.Exec(ctx, "UPDATE payments SET cancelled_at=now(),cancellation_reason=$3 WHERE id=$1 AND meeting_id=$2 AND cancelled_at IS NULL", payment, id, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
		return nil
	})
}
func (s *Store) Close(ctx context.Context, owner, id string) error {
	return s.change(ctx, owner, id, func(tx pgx.Tx, m *Meeting) error {
		if m.State != "finalized" {
			return ErrConflict
		}
		for _, p := range m.Calculation.Totals {
			left, err := remaining(m, p.ParticipantID)
			if err != nil {
				return err
			}
			if left != 0 {
				return fmt.Errorf("%w: остались невозвращённые суммы", ErrConflict)
			}
		}
		_, err := tx.Exec(ctx, "UPDATE meetings SET state='closed',version=version+1 WHERE id=$1", id)
		return err
	})
}
func (s *Store) Link(ctx context.Context, owner, id, participant string, revoke bool) (string, error) {
	token := ""
	err := s.change(ctx, owner, id, func(tx pgx.Tx, m *Meeting) error {
		if m.State == "draft" {
			return ErrConflict
		}
		if _, err := remaining(m, participant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE links SET revoked_at=now() WHERE meeting_id=$1 AND participant_id=$2 AND revoked_at IS NULL", id, participant); err != nil {
			return err
		}
		if revoke {
			return nil
		}
		token = auth.Token()
		_, err := tx.Exec(ctx, "INSERT INTO links(token_hash,meeting_id,participant_id) VALUES($1,$2,$3)", auth.Digest(token), id, participant)
		return err
	})
	return token, err
}

type PersonalLine struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}
type Personal struct {
	Title     string         `json:"title"`
	Date      string         `json:"date"`
	Name      string         `json:"name"`
	State     string         `json:"state"`
	Lines     []PersonalLine `json:"lines"`
	Total     int64          `json:"total"`
	Debt      int64          `json:"debt"`
	Remaining int64          `json:"remaining"`
}

func (s *Store) Personal(ctx context.Context, token string) (Personal, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Personal{}, err
	}
	defer tx.Rollback(ctx)
	var id, participant string
	err = tx.QueryRow(ctx, "SELECT meeting_id::text,participant_id FROM links WHERE token_hash=$1 AND revoked_at IS NULL", auth.Digest(token)).Scan(&id, &participant)
	if errors.Is(err, pgx.ErrNoRows) {
		return Personal{}, ErrNotFound
	}
	if err != nil {
		return Personal{}, err
	}
	m, err := read(tx.QueryRow(ctx, "SELECT "+fields+" FROM meetings WHERE id=$1 AND state<>'draft'", id))
	if err != nil {
		return Personal{}, err
	}
	if err = loadPayments(ctx, tx, &m); err != nil {
		return Personal{}, err
	}
	out := Personal{Title: m.Title, Date: m.Date, State: m.State, Lines: []PersonalLine{}}
	for _, p := range m.Bill.Participants {
		if p.ID == participant {
			out.Name = p.Name
		}
	}
	names := map[string]string{}
	for _, item := range m.Bill.Items {
		names[item.ID] = item.Name
	}
	for _, line := range m.Calculation.Lines {
		for _, share := range line.Shares {
			if share.ParticipantID == participant {
				out.Lines = append(out.Lines, PersonalLine{Name: names[line.ItemID], Amount: share.Amount})
			}
		}
	}
	for _, total := range m.Calculation.Totals {
		if total.ParticipantID == participant {
			out.Total = total.Amount
			out.Debt = *total.Debt
		}
	}
	out.Remaining, err = remaining(&m, participant)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
