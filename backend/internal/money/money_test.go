package money_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"testing"

	"podolyam/internal/demo"
	"podolyam/internal/money"
)

func ptr(n int64) *int64 { return &n }

func participants() []money.Participant {
	return []money.Participant{{ID: "c", Order: 0}, {ID: "b", Order: 1}, {ID: "a", Order: 2}}
}

func weights(values ...int64) []money.Weight {
	p := participants()
	w := make([]money.Weight, len(values))
	for i, v := range values {
		w[i] = money.Weight{ParticipantID: p[i].ID, Value: v}
	}
	return w
}

func bill() money.Bill {
	return money.Bill{
		Participants: participants(), PayerID: "c", ReceiptTotal: ptr(100),
		Items: []money.Item{{ID: "food", Quantity: 1, UnitPrice: ptr(100),
			Assignment: &money.Assignment{Mode: money.Equal, Weights: weights(1, 1, 1)}}},
	}
}

func TestAllocate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		amount  int64
		weights []int64
		want    []int64
	}{
		{"thirds", 100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{"less_than_people", 2, []int64{1, 1, 1}, []int64{1, 1, 0}},
		{"zero", 0, []int64{1, 1, 1}, []int64{0, 0, 0}},
		{"two_to_one", 100, []int64{2, 1}, []int64{67, 33}},
		{"remainder_before_order", 2, []int64{1, 2, 3}, []int64{0, 1, 1}},
		{"one", 99, []int64{1}, []int64{99}},
		{"maximum", money.MaxAmount, []int64{money.MaxFactor, money.MaxFactor, money.MaxFactor}, []int64{33333333334, 33333333333, 33333333333}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := money.Allocate(tc.amount, participants(), weights(tc.weights...))
			if err != nil {
				t.Fatal(err)
			}
			var sum int64
			for i, share := range got {
				if share.Amount != tc.want[i] || share.ParticipantID != participants()[i].ID {
					t.Fatalf("доля %d: %+v; ожидалось %d", i, share, tc.want[i])
				}
				sum += share.Amount
			}
			if sum != tc.amount {
				t.Fatalf("сумма %d", sum)
			}
		})
	}
}

func TestAllocateInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*int64, *[]money.Participant, *[]money.Weight)
	}{
		{"negative", func(a *int64, _ *[]money.Participant, _ *[]money.Weight) { *a = -1 }},
		{"amount_limit", func(a *int64, _ *[]money.Participant, _ *[]money.Weight) { *a = money.MaxAmount + 1 }},
		{"overflow", func(a *int64, _ *[]money.Participant, _ *[]money.Weight) { *a = math.MaxInt64 }},
		{"no_weights", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) { *w = nil }},
		{"zero_weight", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) { (*w)[0].Value = 0 }},
		{"negative_weight", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) { (*w)[0].Value = -1 }},
		{"weight_limit", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) { (*w)[0].Value = math.MaxInt64 }},
		{"duplicate_weight", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) { (*w)[1] = (*w)[0] }},
		{"foreign", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) { (*w)[0].ParticipantID = "other" }},
		{"duplicate_participant", func(_ *int64, p *[]money.Participant, _ *[]money.Weight) { (*p)[1].ID = (*p)[0].ID }},
		{"duplicate_order", func(_ *int64, p *[]money.Participant, _ *[]money.Weight) { (*p)[1].Order = (*p)[0].Order }},
		{"negative_order", func(_ *int64, p *[]money.Participant, _ *[]money.Weight) { (*p)[0].Order = -1 }},
		{"order_limit", func(_ *int64, p *[]money.Participant, _ *[]money.Weight) { (*p)[0].Order = math.MaxInt64 }},
		{"empty_id", func(_ *int64, p *[]money.Participant, _ *[]money.Weight) { (*p)[0].ID = " " }},
		{"participant_limit", func(_ *int64, p *[]money.Participant, _ *[]money.Weight) {
			*p = make([]money.Participant, money.MaxParticipants+1)
		}},
		{"weights_limit", func(_ *int64, _ *[]money.Participant, w *[]money.Weight) {
			*w = make([]money.Weight, money.MaxParticipants+1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, p, w := int64(100), participants(), weights(1, 1, 1)
			tc.change(&a, &p, &w)
			_, err := money.Allocate(a, p, w)
			var detail *money.InputError
			if !errors.Is(err, money.ErrInvalid) || !errors.As(err, &detail) || detail.Field == "" {
				t.Fatalf("нужна ошибка входа: %v", err)
			}
		})
	}
}

func TestModes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     money.Mode
		quantity int64
		w        []int64
		want     []int64
	}{
		{"single", money.Single, 1, []int64{1}, []int64{100, 0, 0}},
		{"equal_shared", money.Equal, 1, []int64{1, 1}, []int64{50, 50, 0}},
		{"all", money.All, 1, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{"units", money.Units, 3, []int64{2, 1}, []int64{200, 100, 0}},
		{"weighted", money.Weighted, 1, []int64{2, 1}, []int64{67, 33, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bill()
			b.Items[0].Quantity = tc.quantity
			b.ReceiptTotal = ptr(100 * tc.quantity)
			b.Items[0].Assignment = &money.Assignment{Mode: tc.mode, Weights: weights(tc.w...)}
			r, err := money.CalculateFinal(b)
			if err != nil {
				t.Fatal(err)
			}
			for i, total := range r.Totals {
				if total.Amount != tc.want[i] {
					t.Fatalf("%+v", r.Totals)
				}
			}
			if *r.Totals[0].Debt != 0 || *r.ToRepay != r.Total-r.Totals[0].Amount {
				t.Fatal("плательщик не должен быть должен самому себе")
			}
		})
	}
}

func TestCalculateInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*money.Bill)
	}{
		{"missing_price", func(b *money.Bill) { b.Items[0].UnitPrice = nil }},
		{"negative_price", func(b *money.Bill) { b.Items[0].UnitPrice = ptr(-1) }},
		{"price_overflow", func(b *money.Bill) { b.Items[0].UnitPrice = ptr(math.MaxInt64) }},
		{"zero_quantity", func(b *money.Bill) { b.Items[0].Quantity = 0 }},
		{"negative_quantity", func(b *money.Bill) { b.Items[0].Quantity = -1 }},
		{"quantity_limit", func(b *money.Bill) { b.Items[0].Quantity = math.MaxInt64 }},
		{"product_limit", func(b *money.Bill) { b.Items[0].Quantity = 2; b.Items[0].UnitPrice = ptr(money.MaxAmount) }},
		{"total_limit", func(b *money.Bill) {
			b.Items[0].UnitPrice = ptr(money.MaxAmount)
			x := b.Items[0]
			x.ID = "second"
			x.UnitPrice = ptr(1)
			b.Items = append(b.Items, x)
		}},
		{"item_limit", func(b *money.Bill) { b.Items = make([]money.Item, money.MaxItems+1) }},
		{"duplicate_item", func(b *money.Bill) { b.Items = append(b.Items, b.Items[0]) }},
		{"empty_item", func(b *money.Bill) { b.Items[0].ID = "" }},
		{"foreign_payer", func(b *money.Bill) { b.PayerID = "other" }},
		{"negative_receipt", func(b *money.Bill) { b.ReceiptTotal = ptr(-1) }},
		{"receipt_limit", func(b *money.Bill) { b.ReceiptTotal = ptr(math.MaxInt64) }},
		{"unknown_mode", func(b *money.Bill) { b.Items[0].Assignment.Mode = "unknown" }},
		{"single_many", func(b *money.Bill) { b.Items[0].Assignment.Mode = money.Single }},
		{"equal_weight", func(b *money.Bill) { b.Items[0].Assignment.Weights[0].Value = 2 }},
		{"units_under", func(b *money.Bill) { b.Items[0].Quantity = 4; b.Items[0].Assignment.Mode = money.Units }},
		{"units_over", func(b *money.Bill) { b.Items[0].Assignment.Mode = money.Units }},
		{"empty_assignment", func(b *money.Bill) { b.Items[0].Assignment.Weights = nil }},
		{"zero_assignment_weight", func(b *money.Bill) { b.Items[0].Assignment.Weights[0].Value = 0 }},
		{"duplicate_assignment", func(b *money.Bill) { b.Items[0].Assignment.Weights[1] = b.Items[0].Assignment.Weights[0] }},
		{"foreign_assignment", func(b *money.Bill) { b.Items[0].Assignment.Weights[0].ParticipantID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bill()
			tc.change(&b)
			_, err := money.Calculate(b)
			if !errors.Is(err, money.ErrInvalid) {
				t.Fatalf("ожидалась валидация: %v", err)
			}
		})
	}
}

func TestDraftAndFinalization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*money.Bill)
		blocker money.Blocker
	}{
		{"unassigned", func(b *money.Bill) { b.Items[0].Assignment = nil }, money.UnassignedItems},
		{"free_unassigned", func(b *money.Bill) {
			b.Items[0].UnitPrice = ptr(0)
			b.ReceiptTotal = ptr(0)
			b.Items[0].Assignment = nil
		}, money.UnassignedItems},
		{"missing_receipt", func(b *money.Bill) { b.ReceiptTotal = nil }, money.NoReceipt},
		{"mismatch", func(b *money.Bill) { b.ReceiptTotal = ptr(99) }, money.ReceiptMismatch},
		{"missing_payer", func(b *money.Bill) { b.PayerID = "" }, money.NoPayer},
		{"empty_items", func(b *money.Bill) { b.Items = nil; b.ReceiptTotal = ptr(0) }, money.NoItems},
		{"empty_bill", func(b *money.Bill) { *b = money.Bill{} }, money.NoParticipants},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bill()
			tc.change(&b)
			r, err := money.Calculate(b)
			if err != nil {
				t.Fatal(err)
			}
			if r.CanFinalize() || !slices.Contains(r.Blockers, tc.blocker) {
				t.Fatalf("%+v", r)
			}
			_, err = money.CalculateFinal(b)
			if !errors.Is(err, money.ErrIncomplete) {
				t.Fatalf("%v", err)
			}
			var assigned int64
			for _, p := range r.Totals {
				assigned += p.Amount
			}
			if assigned+r.UnassignedAmount != r.Total {
				t.Fatal("черновик потерял стоимость")
			}
			if b.PayerID == "" {
				if r.ToRepay != nil {
					t.Fatal("возврат неизвестен без плательщика")
				}
				for _, p := range r.Totals {
					if p.Debt != nil {
						t.Fatal("долг неизвестен")
					}
				}
			}
		})
	}
	b := bill()
	b.Items[0].UnitPrice = ptr(0)
	b.ReceiptTotal = ptr(0)
	if _, err := money.CalculateFinal(b); err != nil {
		t.Fatalf("бесплатная назначенная позиция: %v", err)
	}
}

func TestAllFreezesParticipants(t *testing.T) {
	b := bill()
	a, err := money.AssignAll(b.Participants)
	if err != nil {
		t.Fatal(err)
	}
	b.Items[0].Assignment = &a
	b.Participants = append(b.Participants, money.Participant{ID: "new", Order: 3})
	r, err := money.CalculateFinal(b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals[3].Amount != 0 || len(r.Lines[0].Shares) != 3 {
		t.Fatal("новый участник изменил старые назначения")
	}
	if _, err := money.AssignAll(nil); !errors.Is(err, money.ErrInvalid) {
		t.Fatal("пустой состав")
	}
}

func TestDeterminismAndInputOwnership(t *testing.T) {
	b := bill()
	before, _ := json.Marshal(b)
	first, err := money.CalculateFinal(b)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("изменены входные данные")
	}
	slices.Reverse(b.Participants)
	slices.Reverse(b.Items[0].Assignment.Weights)
	second, err := money.CalculateFinal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("порядок запроса влияет на округление")
	}
	first.Lines[0].Shares[0].Amount = 999
	third, err := money.CalculateFinal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second, third) {
		t.Fatal("результаты разделяют изменяемую память")
	}
}

func TestRestaurant(t *testing.T) {
	b := demo.Restaurant()
	if len(b.Items) != 17 {
		t.Fatal("нужно 17 строк")
	}
	var independent int64
	for _, item := range b.Items {
		independent += item.Quantity * *item.UnitPrice
	}
	if independent != 882000 {
		t.Fatalf("сумма входов %d", independent)
	}
	r, err := money.CalculateFinal(b)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{124168, 152168, 119166, 148166, 172166, 166166}
	var total int64
	for i, p := range r.Totals {
		if p.Amount != want[i] {
			t.Fatalf("%s: %d, ожидалось %d", p.ParticipantID, p.Amount, want[i])
		}
		total += p.Amount
	}
	if total != 882000 || r.Total != 882000 || *r.ToRepay != 757832 || *r.Totals[0].Debt != 0 {
		t.Fatalf("%+v", r)
	}
	for _, line := range r.Lines {
		var sum int64
		for _, s := range line.Shares {
			sum += s.Amount
		}
		if sum != line.Amount {
			t.Fatalf("строка %s потеряла копейки", line.ItemID)
		}
	}
	for _, i := range []int{13, 14} {
		base := []int64{11333, 6333}[i-13]
		for j, s := range r.Lines[i].Shares {
			want := base
			if j < 2 {
				want++
			}
			if s.Amount != want {
				t.Fatalf("строка %d доля %d", i, j)
			}
		}
	}
	other := demo.Restaurant()
	*b.Items[0].UnitPrice = 1
	if *other.Items[0].UnitPrice != 65000 {
		t.Fatal("фикстуры разделяют память")
	}
}

func TestRemaining(t *testing.T) {
	for _, tc := range []struct {
		name     string
		debt     int64
		payments []int64
		want     int64
		bad      bool
	}{
		{"none", 100, nil, 100, false}, {"partial", 100, []int64{30}, 70, false},
		{"full", 100, []int64{30, 70}, 0, false}, {"payer", 0, nil, 0, false},
		{"maximum", money.MaxAmount, []int64{money.MaxAmount}, 0, false},
		{"negative_debt", -1, nil, 0, true}, {"large_debt", math.MaxInt64, nil, 0, true},
		{"zero_payment", 100, []int64{0}, 0, true}, {"negative_payment", 100, []int64{-1}, 0, true},
		{"overpayment", 100, []int64{101}, 0, true}, {"accumulated_overpayment", 100, []int64{60, 41}, 0, true},
		{"payment_overflow", 100, []int64{math.MaxInt64}, 0, true}, {"payer_payment", 0, []int64{1}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := money.Remaining(tc.debt, tc.payments)
			if tc.bad {
				if !errors.Is(err, money.ErrInvalid) {
					t.Fatalf("%v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("%d, %v", got, err)
			}
		})
	}
}

func TestAcceptedLimits(t *testing.T) {
	t.Run("participants", func(t *testing.T) {
		p := make([]money.Participant, money.MaxParticipants)
		w := make([]money.Weight, len(p))
		for i := range p {
			p[i] = money.Participant{ID: fmt.Sprint(i), Order: int64(i)}
			w[i] = money.Weight{ParticipantID: p[i].ID, Value: money.MaxFactor}
		}
		shares, err := money.Allocate(money.MaxAmount, p, w)
		if err != nil {
			t.Fatal(err)
		}
		var total int64
		for _, s := range shares {
			total += s.Amount
		}
		if len(shares) != money.MaxParticipants || total != money.MaxAmount {
			t.Fatal("лимит состава")
		}
	})
	t.Run("quantity", func(t *testing.T) {
		b := bill()
		b.Items[0].Quantity = money.MaxFactor
		b.Items[0].UnitPrice = ptr(money.MaxAmount / money.MaxFactor)
		b.Items[0].Assignment = &money.Assignment{Mode: money.Units, Weights: weights(money.MaxFactor)}
		b.ReceiptTotal = ptr(money.MaxAmount)
		r, err := money.CalculateFinal(b)
		if err != nil || r.Total != money.MaxAmount {
			t.Fatalf("%+v %v", r, err)
		}
	})
	t.Run("items_and_total", func(t *testing.T) {
		b := bill()
		b.Items = make([]money.Item, money.MaxItems)
		for i := range b.Items {
			b.Items[i] = money.Item{
				ID: fmt.Sprint(i), Quantity: 1, UnitPrice: ptr(money.MaxAmount / int64(money.MaxItems)),
				Assignment: &money.Assignment{Mode: money.Single, Weights: weights(1)},
			}
		}
		b.ReceiptTotal = ptr(money.MaxAmount)
		r, err := money.CalculateFinal(b)
		if err != nil || r.Total != money.MaxAmount || len(r.Lines) != money.MaxItems {
			t.Fatalf("лимит строк: %v", err)
		}
	})
}

func TestRejectedResultIsNotFinalizable(t *testing.T) {
	b := bill()
	b.Items[0].UnitPrice = nil
	r, err := money.Calculate(b)
	if err == nil || r.CanFinalize() {
		t.Fatal("ошибочный результат не готов к фиксации")
	}
}
