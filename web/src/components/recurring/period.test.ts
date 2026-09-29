import { describe, expect, it } from 'vitest'
import {
  addMonths,
  dateVars,
  firstOccurrence,
  hasDatePlaceholder,
  insertAt,
  periodLabel,
  renderDatePlaceholders,
  upcomingOccurrences,
} from './period'

describe('periodLabel', () => {
  it('names common periods', () => {
    expect(periodLabel(1)).toBe('měsíčně')
    expect(periodLabel(3)).toBe('čtvrtletně')
    expect(periodLabel(6)).toBe('pololetně')
    expect(periodLabel(12)).toBe('ročně')
  })
  it('declines custom periods', () => {
    expect(periodLabel(2)).toBe('každé 2 měsíce')
    expect(periodLabel(4)).toBe('každé 4 měsíce')
    expect(periodLabel(5)).toBe('každých 5 měsíců')
    expect(periodLabel(18)).toBe('každých 18 měsíců')
    expect(periodLabel(24)).toBe('každé 2 roky')
    expect(periodLabel(60)).toBe('každých 5 let')
  })
})

describe('schedule', () => {
  it('addMonths clamps to month length without drifting', () => {
    expect(addMonths('2026-01-31', 1, 31)).toBe('2026-02-28')
    expect(addMonths('2026-02-28', 1, 31)).toBe('2026-03-31')
    expect(addMonths('2028-01-31', 1)).toBe('2028-02-29')
    expect(addMonths('2026-11-15', 3)).toBe('2027-02-15')
    expect(addMonths('nope', 1)).toBeNull()
  })
  it('firstOccurrence finds the first matching day ≥ start', () => {
    expect(firstOccurrence('2026-09-10', 0)).toBe('2026-09-10')
    expect(firstOccurrence('2026-09-10', 15)).toBe('2026-09-15')
    expect(firstOccurrence('2026-09-20', 15)).toBe('2026-10-15')
    expect(firstOccurrence('2026-02-10', 31)).toBe('2026-02-28')
  })
  it('upcomingOccurrences respects period, anchor and end', () => {
    expect(upcomingOccurrences({ start: '2026-01-31', monthsPeriod: 1 })).toEqual(['2026-01-31', '2026-02-28', '2026-03-31'])
    expect(upcomingOccurrences({ start: '2026-09-20', monthsPeriod: 3, dayOfMonth: 1 })).toEqual([
      '2026-10-01',
      '2027-01-01',
      '2027-04-01',
    ])
    expect(upcomingOccurrences({ start: '2026-09-01', monthsPeriod: 1, endOn: '2026-10-15' })).toEqual([
      '2026-09-01',
      '2026-10-01',
    ])
    expect(upcomingOccurrences({ start: '', monthsPeriod: 1 })).toEqual([])
  })
})

describe('date placeholders', () => {
  it('computes vars like the backend', () => {
    expect(dateVars('2026-01-15')).toMatchObject({
      MONTH: '01',
      MONTH_NAME: 'leden',
      PREV_MONTH: '12',
      PREV_MONTH_NAME: 'prosinec',
      NEXT_MONTH_NAME: 'únor',
      YEAR: '2026',
      PREV_YEAR: '2025',
      QUARTER: '1',
    })
    expect(dateVars('2026-09-30', 'en')?.MONTH_NAME).toBe('September')
    expect(dateVars('2026-13-01')).toBeNull()
  })
  it('renders known placeholders and keeps unknown ones', () => {
    expect(renderDatePlaceholders('Služby za měsíc {MONTH_NAME} {YEAR}', '2026-09-27')).toBe('Služby za měsíc září 2026')
    expect(renderDatePlaceholders('Q{QUARTER}/{YEAR} {FOO} {', '2026-11-01')).toBe('Q4/2026 {FOO} {')
    expect(renderDatePlaceholders('Hosting {PREV_MONTH_NAME}', '2026-09-01', 'en')).toBe('Hosting August')
  })
  it('detects placeholders', () => {
    expect(hasDatePlaceholder('za {MONTH_NAME}')).toBe(true)
    expect(hasDatePlaceholder('za {month}')).toBe(false)
  })
  it('inserts at the caret', () => {
    expect(insertAt('Služby za ', '{MONTH_NAME}')).toEqual({ value: 'Služby za {MONTH_NAME}', caret: 22 })
    expect(insertAt('ab', 'X', 1, 1)).toEqual({ value: 'aXb', caret: 2 })
    expect(insertAt('abc', 'X', 0, 2)).toEqual({ value: 'Xc', caret: 1 })
  })
})
