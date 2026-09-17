// Package demo содержит только явные демоданные разработки.
package demo

import "podolyam/internal/money"

func amount(value int64) *int64 { return &value }

// Restaurant возвращает независимую копию исходного примера из 17 строк.
func Restaurant() money.Bill {
	participants := []money.Participant{
		{ID: "zhenya", Name: "Женя", Order: 0},
		{ID: "dima", Name: "Дима", Order: 1},
		{ID: "kolya", Name: "Коля", Order: 2},
		{ID: "alina", Name: "Алина", Order: 3},
		{ID: "alexey", Name: "Алексей", Order: 4},
		{ID: "beligma", Name: "Бэлигма", Order: 5},
	}
	personal := func(id string) *money.Assignment {
		return &money.Assignment{Mode: money.Single, Weights: []money.Weight{{ParticipantID: id, Value: 1}}}
	}
	pair := func(mode money.Mode, first, second string) *money.Assignment {
		return &money.Assignment{Mode: mode, Weights: []money.Weight{{ParticipantID: first, Value: 1}, {ParticipantID: second, Value: 1}}}
	}
	all := func() *money.Assignment {
		a, err := money.AssignAll(participants)
		if err != nil {
			panic(err)
		} // Ошибка здесь означает неверную встроенную фикстуру.
		return &a
	}
	return money.Bill{
		Participants: participants,
		PayerID:      "zhenya",
		ReceiptTotal: amount(882000),
		Items: []money.Item{
			{ID: "01", Name: "Токпоки Карбонара", Quantity: 1, UnitPrice: amount(65000), Assignment: personal("zhenya")},
			{ID: "02", Name: "Сырный рамен", Quantity: 1, UnitPrice: amount(52000), Assignment: personal("beligma")},
			{ID: "03", Name: "Рамен со свининой", Quantity: 1, UnitPrice: amount(46000), Assignment: personal("dima")},
			{ID: "04", Name: "Том ям с морепродуктами", Quantity: 1, UnitPrice: amount(59000), Assignment: personal("alina")},
			{ID: "05", Name: "Тонкацу", Quantity: 1, UnitPrice: amount(47000), Assignment: personal("dima")},
			{ID: "06", Name: "Удон", Quantity: 1, UnitPrice: amount(41000), Assignment: personal("kolya")},
			{ID: "07", Name: "Паста карбонара", Quantity: 1, UnitPrice: amount(49000), Assignment: personal("alexey")},
			{ID: "08", Name: "Эноки в беконе", Quantity: 1, UnitPrice: amount(38000), Assignment: pair(money.Equal, "kolya", "alexey")},
			{ID: "09", Name: "Лимонад сибирский", Quantity: 2, UnitPrice: amount(25000), Assignment: pair(money.Units, "dima", "alina")},
			{ID: "10", Name: "Лимонад Джекфрут", Quantity: 2, UnitPrice: amount(25000), Assignment: pair(money.Units, "kolya", "beligma")},
			{ID: "11", Name: "Токийский дрифт", Quantity: 1, UnitPrice: amount(60000), Assignment: pair(money.Equal, "alina", "beligma")},
			{ID: "12", Name: "Лимонад малина", Quantity: 1, UnitPrice: amount(25000), Assignment: personal("beligma")},
			{ID: "13", Name: "Джин тоник", Quantity: 1, UnitPrice: amount(45000), Assignment: personal("alexey")},
			{ID: "14", Name: "Гавайская пицца", Quantity: 1, UnitPrice: amount(68000), Assignment: all()},
			{ID: "15", Name: "Чай", Quantity: 1, UnitPrice: amount(38000), Assignment: all()},
			{ID: "16", Name: "Нэко сет", Quantity: 1, UnitPrice: amount(99000), Assignment: all()},
			{ID: "17", Name: "Лимонад апельсин", Quantity: 2, UnitPrice: amount(25000), Assignment: pair(money.Units, "zhenya", "alexey")},
		},
	}
}
