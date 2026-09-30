import { describe, expect, it } from 'vitest'
import { isTypingTarget, matchShortcut, type ShortcutKeyEvent } from './shortcuts'

const body = { tagName: 'BODY' }
const ev = (key: string, extra: Partial<ShortcutKeyEvent> = {}): ShortcutKeyEvent => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  target: body,
  ...extra,
})

describe('isTypingTarget', () => {
  it('detects text fields', () => {
    expect(isTypingTarget({ tagName: 'INPUT', type: 'text' })).toBe(true)
    expect(isTypingTarget({ tagName: 'TEXTAREA' })).toBe(true)
    expect(isTypingTarget({ tagName: 'DIV', isContentEditable: true })).toBe(true)
    expect(isTypingTarget({ tagName: 'DIV', getAttribute: () => 'combobox' })).toBe(true)
  })
  it('ignores non-text elements', () => {
    expect(isTypingTarget({ tagName: 'INPUT', type: 'checkbox' })).toBe(false)
    expect(isTypingTarget({ tagName: 'BUTTON' })).toBe(false)
    expect(isTypingTarget(null)).toBe(false)
  })
})

describe('matchShortcut', () => {
  it('opens the palette with Mod+K even inside inputs', () => {
    expect(matchShortcut(ev('k', { metaKey: true }))).toBe('palette')
    expect(matchShortcut(ev('K', { ctrlKey: true, target: { tagName: 'INPUT' } }))).toBe('palette')
  })
  it('maps single keys outside inputs', () => {
    expect(matchShortcut(ev('/'))).toBe('search')
    expect(matchShortcut(ev('n'))).toBe('new-invoice')
    expect(matchShortcut(ev('?', { shiftKey: true }))).toBe('help')
  })
  it('guards inputs, modifiers, repeats and IME', () => {
    expect(matchShortcut(ev('n', { target: { tagName: 'INPUT' } }))).toBeNull()
    expect(matchShortcut(ev('n', { ctrlKey: true }))).toBeNull()
    expect(matchShortcut(ev('N', { shiftKey: true }))).toBeNull()
    expect(matchShortcut(ev('/', { repeat: true }))).toBeNull()
    expect(matchShortcut(ev('n', { isComposing: true }))).toBeNull()
    expect(matchShortcut(ev('x'))).toBeNull()
  })
})
