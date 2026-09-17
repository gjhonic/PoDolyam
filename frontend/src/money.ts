const formatter = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB' })
export const rubles = (kopecks: number) => formatter.format(kopecks / 100)
export const inputRubles = (kopecks: number | null) => kopecks === null ? '' : `${Math.floor(kopecks / 100)},${String(kopecks % 100).padStart(2, '0')}`

// Это разбор ввода, а не алгоритм распределения. BigInt не даёт потерять
// копейки при преобразовании строки; пустое значение не превращается в ноль.
export function parseRubles(value: string): number {
  const match = /^(\d+)(?:[.,](\d{1,2}))?$/.exec(value.trim())
  if (!match) throw new Error('Введите сумму в рублях, например 250,50')
  const amount = BigInt(match[1]!) * 100n + BigInt((match[2] ?? '').padEnd(2, '0'))
  if (amount > 100000000000n) throw new Error('Сумма превышает допустимый предел')
  return Number(amount)
}
