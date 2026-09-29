import { describe, expect, it } from 'vitest'
import { normalizeReminderDays, reminderSummary } from './reminders'

describe('reminder days', () => {
  it('normalizes like the backend', () => {
    expect(normalizeReminderDays([30, 3, 14, 3, 0, 400, 2.5])).toEqual([3, 14, 30])
  })
  it('summarizes in Czech', () => {
    expect(reminderSummary([3, 14, 30])).toBe('3, 14 a 30 dní po splatnosti')
    expect(reminderSummary([1])).toBe('1 den po splatnosti')
    expect(reminderSummary([7, 3])).toBe('3 a 7 dní po splatnosti')
    expect(reminderSummary([2])).toBe('2 dny po splatnosti')
    expect(reminderSummary([])).toBe('žádná upomínka')
  })
})
