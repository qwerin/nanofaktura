import { describe, expect, it } from 'vitest'
import { todoDueState } from './todo-meta'

describe('todoDueState', () => {
  const today = '2026-09-29'
  it('classifies due dates', () => {
    expect(todoDueState({ completed: false }, today)).toBe('none')
    expect(todoDueState({ due_on: '2026-09-28', completed: false }, today)).toBe('overdue')
    expect(todoDueState({ due_on: '2026-09-29', completed: false }, today)).toBe('today')
    expect(todoDueState({ due_on: '2026-10-01', completed: false }, today)).toBe('soon')
    expect(todoDueState({ due_on: '2026-10-20', completed: false }, today)).toBe('later')
    expect(todoDueState({ due_on: '2026-09-01', completed: true }, today)).toBe('none')
  })
})
