package money

import (
	"sort"
	"strings"
)

func participantIndex(participants []Participant) (map[string]Participant, error) {
	if len(participants) > MaxParticipants {
		return nil, invalid("participants", "слишком много участников")
	}
	index := make(map[string]Participant, len(participants))
	orders := make(map[int64]bool, len(participants))
	for _, p := range participants {
		if strings.TrimSpace(p.ID) == "" {
			return nil, invalid("participants.id", "нужен идентификатор участника")
		}
		if _, exists := index[p.ID]; exists {
			return nil, invalid("participants.id", "участник повторяется")
		}
		if p.Order < 0 || p.Order > MaxAmount || orders[p.Order] {
			return nil, invalid("participants.order", "порядок должен быть уникальным и в диапазоне 0…100000000000")
		}
		index[p.ID] = p
		orders[p.Order] = true
	}
	return index, nil
}

// AssignAll фиксирует текущий состав. Позднейшее добавление участников его не меняет.
func AssignAll(participants []Participant) (Assignment, error) {
	if _, err := participantIndex(participants); err != nil {
		return Assignment{}, err
	}
	if len(participants) == 0 {
		return Assignment{}, invalid("participants", "нужно выбрать участников")
	}
	weights := make([]Weight, len(participants))
	for i, p := range participants {
		weights[i] = Weight{ParticipantID: p.ID, Value: 1}
	}
	return Assignment{Mode: All, Weights: weights}, nil
}

// Allocate делит сумму на положительные целые веса. Результат упорядочен по Order.
func Allocate(amount int64, participants []Participant, weights []Weight) ([]Share, error) {
	index, err := participantIndex(participants)
	if err != nil {
		return nil, err
	}
	return allocate(amount, index, weights)
}

func allocate(amount int64, index map[string]Participant, weights []Weight) ([]Share, error) {
	if amount < 0 || amount > MaxAmount {
		return nil, invalid("amount", "сумма вне допустимого диапазона")
	}
	if len(weights) == 0 || len(weights) > MaxParticipants {
		return nil, invalid("weights", "нужно выбрать от 1 до 1000 участников")
	}
	// Локальный тип нужен только этому алгоритму: наружу выходит простая Share,
	// а порядок и остаток остаются деталями округления.
	type portion struct {
		share     Share
		order     int64
		remainder int64
	}
	portions := make([]portion, len(weights))
	seen := make(map[string]bool, len(weights))
	var totalWeight int64
	for i, weight := range weights {
		p, exists := index[weight.ParticipantID]
		if !exists {
			return nil, invalid("weights.participant_id", "участник не принадлежит встрече")
		}
		if seen[p.ID] {
			return nil, invalid("weights.participant_id", "участник повторяется")
		}
		if weight.Value <= 0 || weight.Value > MaxFactor {
			return nil, invalid("weights.value", "вес должен быть от 1 до 1000000")
		}
		seen[p.ID] = true
		totalWeight += weight.Value // <= 1000 × 1000000.
		portions[i] = portion{share: Share{ParticipantID: p.ID}, order: p.Order}
	}
	// Сначала каждому достаётся целая часть точной дроби. Произведение безопасно:
	// верхние границы amount и weight заданы константами пакета.
	var assigned int64
	for i, weight := range weights {
		product := amount * weight.Value // <= 10^17, проверено лимитами выше.
		portions[i].share.Amount = product / totalWeight
		portions[i].remainder = product % totalWeight
		assigned += portions[i].share.Amount
	}
	// Недостающие копейки получают самые большие дробные остатки. При равенстве
	// стабильный Order делает повторный расчёт детерминированным.
	sort.Slice(portions, func(i, j int) bool {
		if portions[i].remainder != portions[j].remainder {
			return portions[i].remainder > portions[j].remainder
		}
		return portions[i].order < portions[j].order
	})
	for i := int64(0); i < amount-assigned; i++ {
		portions[i].share.Amount++
	}
	// После округления возвращаем бизнес-порядок участников, а не временный
	// порядок, использованный для раздачи остатка.
	sort.Slice(portions, func(i, j int) bool { return portions[i].order < portions[j].order })
	shares := make([]Share, len(portions))
	for i, p := range portions {
		shares[i] = p.share
	}
	return shares, nil
}
