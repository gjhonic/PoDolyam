import { describe, expect, it } from 'vitest'
import { parseRubles, inputRubles } from './money'

describe('Ввод копеек без округления', () => {
  it.each([['0', 0], ['250,50', 25050], ['1.1', 110], ['0002', 200], ['1000000000', 100000000000]])('%s → %s', (text, want) => {
    expect(parseRubles(text)).toBe(want)
    expect(parseRubles(inputRubles(want))).toBe(want)
  })
  it.each(['', ' ', '-1', '1,001', '1e2', 'NaN', '9007199254740992', '1000000000.01'])('отклоняет %s', (text) => {
    expect(() => parseRubles(text)).toThrow()
  })
})
