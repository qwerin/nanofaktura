import { describe, expect, it } from 'vitest'
import {
  defaultVatPeriod,
  formatVatPeriod,
  isVatPeriodClosed,
  parseVatPeriod,
  shiftVatPeriod,
  vatFilingDeadline,
  vatPeriodLabel,
  vatPeriodRange,
} from './vat-period'

describe('parseVatPeriod / formatVatPeriod', () => {
  it.each(['2026-09', '2026-01', '2026-12', '2026-Q1', '2026-Q4'])('round-trips %s', (s) => {
    const p = parseVatPeriod(s)
    expect(p).not.toBeNull()
    expect(formatVatPeriod(p!)).toBe(s)
  })
  it.each(['', '2026-13', '2026-00', '2026-9', '2026-Q5', '2026-q1', 'x'])('rejects %j', (s) => {
    expect(parseVatPeriod(s)).toBeNull()
  })
})

describe('shiftVatPeriod', () => {
  it('crosses year boundaries for months', () => {
    expect(formatVatPeriod(shiftVatPeriod(parseVatPeriod('2026-01')!, -1))).toBe('2025-12')
    expect(formatVatPeriod(shiftVatPeriod(parseVatPeriod('2025-12')!, 1))).toBe('2026-01')
    expect(formatVatPeriod(shiftVatPeriod(parseVatPeriod('2026-03')!, -14))).toBe('2025-01')
  })
  it('crosses year boundaries for quarters', () => {
    expect(formatVatPeriod(shiftVatPeriod(parseVatPeriod('2026-Q1')!, -1))).toBe('2025-Q4')
    expect(formatVatPeriod(shiftVatPeriod(parseVatPeriod('2025-Q4')!, 1))).toBe('2026-Q1')
    expect(formatVatPeriod(shiftVatPeriod(parseVatPeriod('2026-Q2')!, -6))).toBe('2024-Q4')
  })
})

describe('defaultVatPeriod', () => {
  it('is the previous month', () => {
    expect(formatVatPeriod(defaultVatPeriod('month', '2026-09-27'))).toBe('2026-08')
    expect(formatVatPeriod(defaultVatPeriod('month', '2026-01-05'))).toBe('2025-12')
  })
  it('is the previous quarter', () => {
    expect(formatVatPeriod(defaultVatPeriod('quarter', '2026-09-27'))).toBe('2026-Q2')
    expect(formatVatPeriod(defaultVatPeriod('quarter', '2026-10-01'))).toBe('2026-Q3')
    expect(formatVatPeriod(defaultVatPeriod('quarter', '2026-02-10'))).toBe('2025-Q4')
  })
})

describe('ranges, labels and deadlines', () => {
  it('computes ranges incl. leap years', () => {
    expect(vatPeriodRange(parseVatPeriod('2028-02')!)).toEqual({ from: '2028-02-01', to: '2028-02-29' })
    expect(vatPeriodRange(parseVatPeriod('2026-Q3')!)).toEqual({ from: '2026-07-01', to: '2026-09-30' })
    expect(vatPeriodRange(parseVatPeriod('2026-Q4')!)).toEqual({ from: '2026-10-01', to: '2026-12-31' })
  })
  it('labels in Czech', () => {
    expect(vatPeriodLabel(parseVatPeriod('2026-09')!)).toBe('září 2026')
    expect(vatPeriodLabel(parseVatPeriod('2026-Q3')!)).toBe('3. čtvrtletí 2026')
  })
  it('deadline is the 25th of the following month', () => {
    expect(vatFilingDeadline(parseVatPeriod('2026-08')!)).toBe('2026-09-25')
    expect(vatFilingDeadline(parseVatPeriod('2026-12')!)).toBe('2027-01-25')
    expect(vatFilingDeadline(parseVatPeriod('2026-Q3')!)).toBe('2026-10-25')
  })
  it('knows whether a period is over', () => {
    expect(isVatPeriodClosed(parseVatPeriod('2026-08')!, '2026-09-01')).toBe(true)
    expect(isVatPeriodClosed(parseVatPeriod('2026-09')!, '2026-09-27')).toBe(false)
  })
})
