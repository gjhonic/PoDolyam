package money

import (
	"fmt"
	"sort"
	"strings"
)

// Calculate разрешает неполный черновик, но отклоняет некорректные входы.
// Нераспределённые строки включаются в Total и остаются в UnassignedIDs.
func Calculate(bill Bill) (Result, error) {
	index, err := participantIndex(bill.Participants)
	if err != nil {
		return Result{}, err
	}
	if bill.PayerID != "" {
		if _, exists := index[bill.PayerID]; !exists {
			return Result{}, invalid("payer_id", "плательщик не принадлежит встрече")
		}
	}
	if len(bill.Items) > MaxItems {
		return Result{}, invalid("items", "слишком много позиций")
	}
	if bill.ReceiptTotal != nil && (*bill.ReceiptTotal < 0 || *bill.ReceiptTotal > MaxAmount) {
		return Result{}, invalid("receipt_total", "сумма чека вне допустимого диапазона")
	}
	// Непустые срезы создаются заранее, чтобы JSON содержал [] вместо null.
	// Для frontend это избавляет от отдельных проверок на отсутствующий массив.
	result := Result{
		Algorithm:     AlgorithmVersion,
		Lines:         make([]Line, 0, len(bill.Items)),
		Totals:        make([]ParticipantTotal, 0, len(index)),
		UnassignedIDs: []string{},
		Blockers:      []Blocker{},
	}
	totals := make(map[string]int64, len(index))
	seen := make(map[string]bool, len(bill.Items))
	for i, item := range bill.Items {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			return Result{}, invalid("items.id", "идентификатор позиции должен быть непустым и уникальным")
		}
		seen[item.ID] = true
		amount, err := lineAmount(item)
		if err != nil {
			return Result{}, fmt.Errorf("позиция %d: %w", i+1, err)
		}
		if amount > MaxAmount-result.Total {
			return Result{}, invalid("items", "сумма встречи превышает лимит")
		}
		result.Total += amount
		line := Line{ItemID: item.ID, Amount: amount, Shares: []Share{}}
		if item.Assignment == nil {
			result.UnassignedAmount += amount
			result.UnassignedIDs = append(result.UnassignedIDs, item.ID)
		} else {
			if err := validateMode(item); err != nil {
				return Result{}, fmt.Errorf("позиция %d: %w", i+1, err)
			}
			line.Shares, err = allocate(amount, index, item.Assignment.Weights)
			if err != nil {
				return Result{}, fmt.Errorf("позиция %d: %w", i+1, err)
			}
			for _, share := range line.Shares {
				totals[share.ParticipantID] += share.Amount
			}
		}
		result.Lines = append(result.Lines, line)
	}
	// append в новый nil-срез копирует элементы. Так сортировка не меняет
	// входной Bill: срезы в Go могут разделять один backing array.
	ordered := append([]Participant(nil), bill.Participants...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Order < ordered[j].Order })
	var repay int64
	for _, p := range ordered {
		total := ParticipantTotal{ParticipantID: p.ID, Amount: totals[p.ID]}
		if bill.PayerID != "" {
			debt := total.Amount
			if p.ID == bill.PayerID {
				debt = 0
			}
			total.Debt = &debt
			repay += debt
		}
		result.Totals = append(result.Totals, total)
	}
	if bill.PayerID != "" {
		result.ToRepay = &repay
	}
	// Blockers описывают допустимый, но незавершённый черновик. Ошибка выше
	// означает некорректные данные, а blocker — недостающий шаг пользователя.
	if len(index) == 0 {
		result.Blockers = append(result.Blockers, NoParticipants)
	}
	if bill.PayerID == "" {
		result.Blockers = append(result.Blockers, NoPayer)
	}
	if len(bill.Items) == 0 {
		result.Blockers = append(result.Blockers, NoItems)
	}
	if bill.ReceiptTotal == nil {
		result.Blockers = append(result.Blockers, NoReceipt)
	} else if *bill.ReceiptTotal != result.Total {
		result.Blockers = append(result.Blockers, ReceiptMismatch)
	}
	if len(result.UnassignedIDs) != 0 {
		result.Blockers = append(result.Blockers, UnassignedItems)
	}
	return result, nil
}

func lineAmount(item Item) (int64, error) {
	if item.Quantity <= 0 || item.Quantity > MaxFactor {
		return 0, invalid("items.quantity", "количество должно быть от 1 до 1000000")
	}
	// Указатель отличает «поле не заполнено» от явно введённой нулевой цены.
	if item.UnitPrice == nil {
		return 0, invalid("items.unit_price", "введите цену, включая 0 для бесплатной позиции")
	}
	price := *item.UnitPrice
	if price < 0 || price > MaxAmount/item.Quantity {
		return 0, invalid("items.unit_price", "цена отрицательная или стоимость строки превышает лимит")
	}
	return price * item.Quantity, nil
}

func validateMode(item Item) error {
	a := item.Assignment
	switch a.Mode {
	case Single:
		if len(a.Weights) != 1 {
			return invalid("assignment", "личная позиция назначается одному участнику")
		}
	case Equal, All, Units, Weighted:
	default:
		return invalid("assignment.mode", "неизвестный режим распределения")
	}
	if len(a.Weights) == 0 || len(a.Weights) > MaxParticipants {
		return invalid("assignment.weights", "нужно выбрать от 1 до 1000 участников")
	}
	var units int64
	for _, weight := range a.Weights {
		if weight.Value <= 0 || weight.Value > MaxFactor {
			return invalid("assignment.weights", "вес должен быть от 1 до 1000000")
		}
		if (a.Mode == Single || a.Mode == Equal || a.Mode == All) && weight.Value != 1 {
			return invalid("assignment.weights", "для равного распределения вес должен быть 1")
		}
		units += weight.Value
	}
	if a.Mode == Units && units != item.Quantity {
		return invalid("assignment.weights", "число распределённых единиц не равно количеству позиции")
	}
	return nil
}
