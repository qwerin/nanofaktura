import { describe, expect, it } from 'vitest'
import {
  addDays,
  daysBetween,
  formatDate,
  formatDateLong,
  formatDueRelative,
  parseISODate,
  todayISO,
} from './date'

describe('parseISODate', () => {
  it('parses valid dates', () => {
    expect(parseISODate('2026-09-26')?.toISOString()).toBe('2026-09-26T00:00:00.000Z')
  })
  it.each(['', '2026-9-26', '2026-02-30', '2026-13-01', 'abc'])('rejects %j', (s) => {
    expect(parseISODate(s)).toBeNull()
  })
})

describe('formatDate', () => {
  it('formats in Czech', () => {
    expect(formatDate('2026-09-26').replace(/\u00a0/g, ' ')).toBe('26. 9. 2026')
    expect(formatDateLong('2026-09-26').replace(/\u00a0/g, ' ')).toBe('26. září 2026')
  })
  it('handles empty', () => {
    expect(formatDate('')).toBe('')
    expect(formatDate(null)).toBe('')
    expect(formatDate('nope')).toBe('')
  })
})

describe('date arithmetic', () => {
  it('adds days across months and leap years', () => {
    expect(addDays('2026-01-30', 14)).toBe('2026-02-13')
    expect(addDays('2028-02-28', 1)).toBe('2028-02-29')
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28')
  })
  it('counts days between', () => {
    expect(daysBetween('2026-09-26', '2026-10-10')).toBe(14)
    expect(daysBetween('2026-10-10', '2026-09-26')).toBe(-14)
    // přes změnu času (DST) se nic neposune — počítáme v UTC
    expect(daysBetween('2026-03-28', '2026-03-30')).toBe(2)
  })
  it('todayISO uses local date parts', () => {
    expect(todayISO(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05')
  })
})

describe('formatDueRelative', () => {
  const today = '2026-09-26'
  it.each([
    ['2026-09-26', 'dnes'],
    ['2026-09-27', 'zítra'],
    ['2026-09-25', 'včera'],
    ['2026-09-29', 'za 3 dny'],
    ['2026-10-06', 'za 10 dní'],
    ['2026-09-21', 'před 5 dny'],
  ])('%s → %s', (due, expected) => {
    expect(formatDueRelative(due, today)).toBe(expected)
  })
})
