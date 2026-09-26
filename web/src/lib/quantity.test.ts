import { describe, expect, it } from 'vitest'
import { formatQuantity, lineAmount, milliToQuantity, parseQuantity, quantityToMilli } from './quantity'

const norm = (s: string) => s.replace(/[\u00a0\u202f]/g, ' ')

describe('parseQuantity', () => {
  it.each([
    ['1', '1'],
    ['1,5', '1.5'],
    [' 1.500 ', '1.5'],
    ['-0,25', '-0.25'],
    ['007', '7'],
    ['0,000', '0'],
    ['-0', '0'],
    ['.5', '0.5'],
    ['1 000', '1000'],
    ['2.125', '2.125'],
  ])('%j → %j', (input, expected) => {
    expect(parseQuantity(input)).toBe(expected)
  })

  it.each(['', '-', 'abc', '1.2345', '1,2,3', '1e3'])('rejects %j', (input) => {
    expect(parseQuantity(input)).toBeNull()
  })
})

describe('quantity milli conversion', () => {
  it.each([
    ['1.5', 1500],
    ['-0.25', -250],
    ['2.125', 2125],
    ['10', 10000],
  ])('%s ↔ %i', (q, milli) => {
    expect(quantityToMilli(q)).toBe(milli)
    expect(milliToQuantity(milli)).toBe(q)
  })

  it('throws on invalid', () => {
    expect(() => quantityToMilli('x')).toThrow()
  })
})

describe('formatQuantity', () => {
  it.each([
    ['1.5', '1,5'],
    ['1234', '1 234'],
    ['-0.125', '-0,125'],
  ])('%s → %s', (q, expected) => {
    expect(norm(formatQuantity(q))).toBe(expected)
  })
})

describe('lineAmount', () => {
  it.each([
    [10000, '1', 10000],
    [10000, '1.5', 15000],
    [333, '0.5', 167], // 166.5 → half away → 167
    [333, '-0.5', -167],
    [1, '0.001', 0],
    [500, '0.001', 1], // 0.5 → 1
  ])('%i × %s → %i', (price, q, expected) => {
    expect(lineAmount(price, q)).toBe(expected)
  })
})
