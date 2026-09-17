// Package money рассчитывает доли ресторанного счёта целыми копейками.
// Пакет не выполняет ввод-вывод и не изменяет переданные данные.
package money

import (
	"errors"
	"fmt"
)

const (
	MaxAmount        int64 = 100_000_000_000
	MaxFactor        int64 = 1_000_000
	MaxParticipants        = 1000
	MaxItems               = 10000
	AlgorithmVersion       = "largest-remainder-v1"
)

var (
	ErrInvalid    = errors.New("некорректные данные расчёта")
	ErrIncomplete = errors.New("расчёт не готов к фиксации")
)

type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return e.Field + ": " + e.Message }
func (e *InputError) Unwrap() error { return ErrInvalid }

func invalid(field, message string) error {
	return &InputError{Field: field, Message: message}
}

type Participant struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int64  `json:"order"`
}

type Weight struct {
	ParticipantID string `json:"participant_id"`
	Value         int64  `json:"value"`
}

type Mode string

const (
	Single   Mode = "single"
	Equal    Mode = "equal"
	All      Mode = "all"
	Units    Mode = "units"
	Weighted Mode = "weighted"
)

// Assignment хранит выбранный состав, в том числе для режима All.
// Для Single/Equal/All значение каждого веса должно быть 1.
type Assignment struct {
	Mode    Mode     `json:"mode"`
	Weights []Weight `json:"weights"`
}

type Item struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Quantity   int64       `json:"quantity"`
	UnitPrice  *int64      `json:"unit_price"`
	Assignment *Assignment `json:"assignment"`
}

type Bill struct {
	Participants []Participant `json:"participants"`
	PayerID      string        `json:"payer_id"`
	Items        []Item        `json:"items"`
	ReceiptTotal *int64        `json:"receipt_total"`
}

type Share struct {
	ParticipantID string `json:"participant_id"`
	Amount        int64  `json:"amount"`
}

type Line struct {
	ItemID string  `json:"item_id"`
	Amount int64   `json:"amount"`
	Shares []Share `json:"shares"`
}

type ParticipantTotal struct {
	ParticipantID string `json:"participant_id"`
	Amount        int64  `json:"amount"`
	// Debt неизвестен (nil), пока не выбран плательщик.
	Debt *int64 `json:"debt"`
}

type Blocker string

const (
	NoParticipants  Blocker = "no_participants"
	NoPayer         Blocker = "no_payer"
	NoItems         Blocker = "no_items"
	NoReceipt       Blocker = "no_receipt"
	ReceiptMismatch Blocker = "receipt_mismatch"
	UnassignedItems Blocker = "unassigned_items"
)

type Result struct {
	Algorithm        string             `json:"algorithm"`
	Lines            []Line             `json:"lines"`
	Totals           []ParticipantTotal `json:"totals"`
	Total            int64              `json:"total"`
	UnassignedAmount int64              `json:"unassigned_amount"`
	UnassignedIDs    []string           `json:"unassigned_ids"`
	ToRepay          *int64             `json:"to_repay"`
	Blockers         []Blocker          `json:"blockers"`
}

func (r Result) CanFinalize() bool { return r.Algorithm == AlgorithmVersion && len(r.Blockers) == 0 }

// CalculateFinal проверяет полноту, но не сохраняет снимок.
// Транзакционная фиксация и защита снимка от изменений принадлежат слою хранения.
func CalculateFinal(bill Bill) (Result, error) {
	result, err := Calculate(bill)
	if err != nil {
		return Result{}, err
	}
	if !result.CanFinalize() {
		return Result{}, fmt.Errorf("%w: %v", ErrIncomplete, result.Blockers)
	}
	return result, nil
}
