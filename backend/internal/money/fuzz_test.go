package money_test

import (
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"slices"
	"testing"

	"podolyam/internal/money"
)

func FuzzAllocate(f *testing.F) {
	f.Add(uint64(100), []byte{1, 1, 1})
	f.Add(uint64(money.MaxAmount), []byte{255, 255, 255})
	f.Add(uint64(0), []byte{0})
	f.Fuzz(func(t *testing.T, raw uint64, data []byte) {
		if len(data) == 0 {
			return
		}
		if len(data) > 64 {
			data = data[:64]
		}
		amount := int64(raw % uint64(money.MaxAmount+1))
		p := make([]money.Participant, len(data))
		w := make([]money.Weight, len(data))
		var totalWeight int64
		for i, v := range data {
			p[i] = money.Participant{ID: fmt.Sprint(i), Order: int64(i)}
			w[i] = money.Weight{ParticipantID: p[i].ID, Value: int64(v)*3921 + 1}
			totalWeight += w[i].Value
		}
		shares, err := money.Allocate(amount, p, w)
		if err != nil {
			t.Fatal(err)
		}
		if len(shares) != len(w) {
			t.Fatal("не все участники учтены")
		}
		var sum int64
		for i, s := range shares {
			sum += s.Amount
			// Независимая арифметика произвольной точности проверяет границы округления.
			product := new(big.Int).Mul(big.NewInt(amount), big.NewInt(w[i].Value))
			base, rem := new(big.Int), new(big.Int)
			base.QuoRem(product, big.NewInt(totalWeight), rem)
			if s.ParticipantID != p[i].ID || s.Amount < base.Int64() || s.Amount > base.Int64()+1 || (rem.Sign() == 0 && s.Amount != base.Int64()) {
				t.Fatalf("неверная доля: %+v", s)
			}
		}
		if sum != amount {
			t.Fatalf("сумма %d != %d", sum, amount)
		}
		again, err := money.Allocate(amount, p, w)
		if err != nil || !reflect.DeepEqual(shares, again) {
			t.Fatal("нет детерминированности")
		}
		slices.Reverse(p)
		slices.Reverse(w)
		reversed, err := money.Allocate(amount, p, w)
		if err != nil || !reflect.DeepEqual(shares, reversed) {
			t.Fatal("порядок входа влияет на доли")
		}
	})
}

func FuzzCalculate(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Add([]byte{255, 254, 253})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		if len(data) > 32 {
			data = data[:32]
		}
		b := bill()
		b.Items = nil
		var expected, unassigned int64
		var unassignedCount int
		for i, v := range data {
			price := int64(v) * 123457
			quantity := int64(v%7) + 1
			item := money.Item{ID: fmt.Sprint(i), Quantity: quantity, UnitPrice: ptr(price)}
			if v%3 != 0 {
				item.Assignment = &money.Assignment{Mode: money.Weighted, Weights: weights(int64(v)+1, 1, 2)}
			} else {
				unassigned += price * quantity
				unassignedCount++
			}
			b.Items = append(b.Items, item)
			expected += price * quantity
		}
		b.ReceiptTotal = ptr(expected)
		r, err := money.Calculate(b)
		if err != nil {
			t.Fatal(err)
		}
		var assigned int64
		for _, p := range r.Totals {
			assigned += p.Amount
		}
		if r.Total != expected || assigned+r.UnassignedAmount != expected || r.UnassignedAmount != unassigned || len(r.UnassignedIDs) != unassignedCount {
			t.Fatal("не сохранена сумма черновика")
		}
		if r.CanFinalize() != (unassignedCount == 0) {
			t.Fatal("неверная готовность")
		}
		if *r.ToRepay != assigned-r.Totals[0].Amount {
			t.Fatal("неверный долг плательщику")
		}
		again, err := money.Calculate(b)
		if err != nil || !reflect.DeepEqual(r, again) {
			t.Fatal("нет детерминированности")
		}
		for _, line := range r.Lines {
			if len(line.Shares) == 0 {
				continue
			}
			var sum int64
			for _, s := range line.Shares {
				sum += s.Amount
			}
			if sum != line.Amount {
				t.Fatal("сумма долей строки не равна стоимости")
			}
		}
	})
}

func FuzzInvalidNumbers(f *testing.F) {
	f.Add(int64(100), int64(1))
	f.Add(int64(-1), int64(1))
	f.Add(int64(9223372036854775807), int64(9223372036854775807))
	f.Fuzz(func(t *testing.T, amount, weight int64) {
		got, err := money.Allocate(amount, participants(), weights(weight))
		valid := amount >= 0 && amount <= money.MaxAmount && weight > 0 && weight <= money.MaxFactor
		if valid {
			if err != nil || len(got) != 1 || got[0].Amount != amount {
				t.Fatalf("допустимый вход: %v %v", got, err)
			}
		} else if !errors.Is(err, money.ErrInvalid) {
			t.Fatalf("недопустимый вход принят: %d %d", amount, weight)
		}
	})
}
