import { describe, expect, it } from 'vitest'
import { insertAt, numberFormatError, numberFormatPeriod, renderNumberFormat } from './number-format'

const date = new Date(2026, 2, 5) // 5. 3. 2026

describe('renderNumberFormat', () => {
  it.each([
    ['{YYYY}-{NNNN}', 7, '2026-0007'],
    ['Z{YYYY}-{NNNN}', 1, 'Z2026-0001'],
    ['{YY}{MM}{NNN}', 42, '2603042'],
    ['FV-{N}', 12345, 'FV-12345'],
    ['{NN}', 123, '123'],
  ])('%s #%d → %s', (format, n, expected) => {
    expect(renderNumberFormat(format, date, n)).toBe(expected)
  })
  it('returns null for invalid formats', () => {
    expect(renderNumberFormat('{YYYY}', date, 1)).toBeNull()
  })
})

describe('numberFormatError', () => {
  it('accepts valid formats', () => {
    expect(numberFormatError('{YYYY}-{NNNN}')).toBeNull()
    expect(numberFormatError('{NNNNNN}')).toBeNull()
  })
  it.each([
    ['', 'Zadejte formát'],
    ['{YYYY}', 'pořadové číslo'],
    ['{NNNNNNN}', 'Neznámý'],
    ['{YYYY-{NN}', 'Neznámý'],
    ['{NNNN', 'Neuzavřená'],
    ['NN}', 'Nadbytečná'],
    ['{Q}{N}', 'Neznámý zástupný symbol {Q}'],
    ['x'.repeat(51) + '{N}', 'Nejvýše'],
  ])('rejects %j', (format, msg) => {
    expect(numberFormatError(format)).toContain(msg)
  })
})

describe('numberFormatPeriod', () => {
  it('derives the reset period', () => {
    expect(numberFormatPeriod('{YYYY}{MM}{NN}')).toBe('month')
    expect(numberFormatPeriod('{YY}-{NN}')).toBe('year')
    expect(numberFormatPeriod('F{NNNN}')).toBe('never')
    expect(numberFormatPeriod('{X}')).toBeNull()
  })
})

describe('insertAt', () => {
  it('inserts at the cursor and replaces a selection', () => {
    expect(insertAt('AB', '{N}', 1)).toEqual({ value: 'A{N}B', cursor: 4 })
    expect(insertAt('AXXB', '{MM}', 1, 3)).toEqual({ value: 'A{MM}B', cursor: 5 })
    expect(insertAt('', '{YYYY}', 5)).toEqual({ value: '{YYYY}', cursor: 6 })
  })
})
