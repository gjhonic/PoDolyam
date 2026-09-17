// Package meetings описывает встречу и проверяет изменяемый черновик.
package meetings

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

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
	// Встроенное поле Draft продвигает его поля в Meeting и сохраняет плоский JSON.
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

func ValidateDraft(d Draft) error {
	if strings.TrimSpace(d.Title) == "" || utf8.RuneCountInString(d.Title) > 120 || utf8.RuneCountInString(d.Description) > 1000 || utf8.RuneCountInString(d.Venue) > 200 {
		return ErrInvalid
	}
	date, err := time.Parse(time.DateOnly, d.Date)
	if err != nil || date.Year() < 1900 || date.Year() > 9999 {
		return ErrInvalid
	}
	for _, participant := range d.Bill.Participants {
		if strings.TrimSpace(participant.Name) == "" || utf8.RuneCountInString(participant.Name) > 80 {
			return ErrInvalid
		}
	}
	for _, item := range d.Bill.Items {
		if strings.TrimSpace(item.Name) == "" || utf8.RuneCountInString(item.Name) > 160 {
			return ErrInvalid
		}
	}
	// Денежный пакет остаётся единственным местом полной проверки счёта.
	_, err = money.Calculate(d.Bill)
	return err
}
