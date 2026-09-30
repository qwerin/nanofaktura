// Globální klávesové zkratky (desktop, SPEC §7.15). Čistá logika bez DOM závislostí → testovatelná.

export type ShortcutAction = 'palette' | 'search' | 'new-invoice' | 'help'

/** Minimum z `KeyboardEvent`, které guard potřebuje. */
export interface ShortcutKeyEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey?: boolean
  repeat?: boolean
  isComposing?: boolean
  target: unknown
}

interface TargetLike {
  tagName?: string
  isContentEditable?: boolean
  getAttribute?: (name: string) => string | null
  type?: string
}

const TEXT_ROLES = new Set(['textbox', 'combobox', 'searchbox', 'spinbutton'])
const NON_TEXT_INPUTS = new Set(['checkbox', 'radio', 'button', 'submit', 'reset', 'file', 'range', 'color'])

/** Píše uživatel právě do pole? (input, textarea, select, contenteditable, role=textbox …) */
export function isTypingTarget(target: unknown): boolean {
  if (!target || typeof target !== 'object') return false
  const t = target as TargetLike
  const tag = t.tagName?.toUpperCase()
  if (tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (tag === 'INPUT') return !NON_TEXT_INPUTS.has((t.type ?? 'text').toLowerCase())
  if (t.isContentEditable) return true
  const role = t.getAttribute?.('role')
  return role ? TEXT_ROLES.has(role) : false
}

/**
 * Která akce odpovídá stisku klávesy, nebo `null`.
 * - ⌘K / Ctrl+K → paleta (i uvnitř polí)
 * - „/“ → hledání, „n“ → nová faktura, „?“ → nápověda — jen mimo pole a bez modifikátorů
 */
export function matchShortcut(e: ShortcutKeyEvent): ShortcutAction | null {
  if (e.isComposing) return null
  const key = e.key.length === 1 ? e.key.toLowerCase() : e.key
  if ((e.metaKey || e.ctrlKey) && !e.altKey && key === 'k') return 'palette'
  if (e.metaKey || e.ctrlKey || e.altKey || e.repeat) return null
  if (isTypingTarget(e.target)) return null
  if (key === '/') return 'search'
  if (key === '?') return 'help'
  if (key === 'n' && !e.shiftKey) return 'new-invoice'
  return null
}

/** Popisy pro dialog nápovědy. `mod` = ⌘ na macOS, jinak Ctrl. */
export function shortcutList(isMac: boolean): { keys: string[]; label: string }[] {
  const mod = isMac ? '⌘' : 'Ctrl'
  return [
    { keys: [mod, 'K'], label: 'Hledat a spouštět akce' },
    { keys: ['/'], label: 'Hledat' },
    { keys: ['N'], label: 'Nová faktura' },
    { keys: ['?'], label: 'Klávesové zkratky' },
    { keys: [mod, 'B'], label: 'Sbalit / rozbalit postranní panel' },
    { keys: ['Esc'], label: 'Zavřít dialog' },
  ]
}

export function isMacPlatform(): boolean {
  if (typeof navigator === 'undefined') return false
  return /mac|iphone|ipad/i.test(navigator.platform || navigator.userAgent)
}
