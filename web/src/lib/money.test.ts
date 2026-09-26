import { describe, expect, it } from 'vitest'
import { divRoundHalfAway, formatMoney, formatMoneyInput, formatVatRate, parseMoney } from './money'

// Intl v cs-CZ používá jako oddělovač tisíců i před měnou nezlomitelnou mezeru.
const norm = (s: string) => s.replace(/[\u00a0\u202f]/g, ' ')

describe('formatMoney', () => {
  it.each([
    [123450, 'CZK', '1 234,50 Kč'],
    [0, 'CZK', '0,00 Kč'],
    [1, 'CZK', '0,01 Kč'],
    [-99, 'CZK', '-0,99 Kč'],
    [100000000, 'CZK', '1 000 000,00 Kč'],
    [1050, 'EUR', '10,50 €'],
  ])('%i %s → %s', (amount, currency, expected) => {
    expect(norm(formatMoney(amount, currency))).toBe(expected)
  })

  it('defaults to CZK', () => {
    expect(norm(formatMoney(100))).toBe('1,00 Kč')
  })

  it('hides zero decimals on request', () => {
    expect(norm(formatMoney(150000, 'CZK', { hideZeroDecimals: true }))).toBe('1 500 Kč')
    expect(norm(formatMoney(150050, 'CZK', { hideZeroDecimals: true }))).toBe('1 500,50 Kč')
  })
})

describe('parseMoney', () => {
  it.each([
    ['1 234,50', 123450],
    ['1\u00a0234,50\u00a0Kč', 123450],
    ['1234.5', 123450],
    ['1234', 123400],
    ['0,01', 1],
    [',5', 50],
    ['-12,30', -1230],
    ['\u221212,30', -1230],
    ['+7', 700],
    ['1.234,56', 123456],
    ['1,234.56', 123456],
    ['1.234.567', 123456700],
    ['12 CZK', 1200],
    ['-0', 0],
  ])('%j → %i', (input, expected) => {
    expect(parseMoney(input)).toBe(expected)
  })

  it.each(['', '  ', 'abc', '1,234', '1,2,3', '12,345', '1.2.3', '--1', '1e5', '12,3x'])(
    'rejects %j',
    (input) => {
      // '1,234' má 3 desetinná místa → neplatné (tisíce by musely mít i další skupinu)
      expect(parseMoney(input)).toBeNull()
    },
  )
})

describe('formatMoneyInput', () => {
  it.each([
    [123450, '1234,50'],
    [5, '0,05'],
    [-1230, '-12,30'],
    [0, '0,00'],
  ])('%i → %s', (amount, expected) => {
    expect(formatMoneyInput(amount)).toBe(expected)
    expect(parseMoney(formatMoneyInput(amount))).toBe(amount)
  })
})

describe('divRoundHalfAway', () => {
  it.each([
    [5n, 2n, 3n],
    [-5n, 2n, -3n],
    [4n, 3n, 1n],
    [-4n, 3n, -1n],
    [15n, 10n, 2n],
    [14n, 10n, 1n],
    [5n, -2n, -3n],
  ])('%s / %s → %s', (n, d, expected) => {
    expect(divRoundHalfAway(n, d)).toBe(expected)
  })
})

describe('formatVatRate', () => {
  it.each([
    [2100, '21 %'],
    [1200, '12 %'],
    [0, '0 %'],
    [1250, '12,5 %'],
  ])('%i → %s', (bps, expected) => {
    expect(norm(formatVatRate(bps))).toBe(expected)
  })
})
