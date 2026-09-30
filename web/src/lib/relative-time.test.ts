import { describe, expect, it } from 'vitest'
import { formatRelativeTime } from './relative-time'

const now = new Date(2026, 8, 29, 15, 0, 0)
const ago = (ms: number) => new Date(now.getTime() - ms).toISOString()
const MIN = 60_000
const H = 60 * MIN

describe('formatRelativeTime', () => {
  it('handles empty and invalid input', () => {
    expect(formatRelativeTime(undefined, now)).toBe('')
    expect(formatRelativeTime('nope', now)).toBe('')
  })
  it('uses "právě teď" for recent and slightly future instants', () => {
    expect(formatRelativeTime(ago(10_000), now)).toBe('právě teď')
    expect(formatRelativeTime(ago(-5_000), now)).toBe('právě teď')
  })
  it('formats minutes and hours', () => {
    expect(formatRelativeTime(ago(5 * MIN), now)).toBe('před 5 min')
    expect(formatRelativeTime(ago(2 * H), now)).toBe('před 2 h')
  })
  it('formats yesterday with time and days', () => {
    expect(formatRelativeTime(new Date(2026, 8, 28, 9, 5).toISOString(), now)).toBe('včera 9:05')
    expect(formatRelativeTime(new Date(2026, 8, 26, 12, 0).toISOString(), now)).toBe('před 3 dny')
  })
  it('falls back to a date after a week', () => {
    expect(formatRelativeTime(new Date(2026, 8, 1, 12, 0).toISOString(), now)).toBe('1. 9.')
    expect(formatRelativeTime(new Date(2025, 11, 24, 12, 0).toISOString(), now)).toBe('24. 12. 2025')
  })
})
