package money

// Remaining считает остаток по неотменённым получениям.
// Аудит отмены и защита от конкурентной переплаты реализуются в хранилище.
func Remaining(debt int64, receipts []int64) (int64, error) {
	if debt < 0 || debt > MaxAmount {
		return 0, invalid("debt", "обязательство вне допустимого диапазона")
	}
	left := debt
	for _, amount := range receipts {
		if amount <= 0 {
			return 0, invalid("receipts", "сумма получения должна быть положительной")
		}
		if amount > left {
			return 0, invalid("receipts", "получения превышают обязательство")
		}
		left -= amount
	}
	return left, nil
}
