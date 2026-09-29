// Vkládání placeholderů ({MONTH_NAME}, {number}…) na pozici kurzoru v posledním aktivním poli formuláře.

import { useRef, useState, type FocusEvent } from 'react'
import { insertAt } from './period'

type TextEl = HTMLInputElement | HTMLTextAreaElement

interface Options {
  /** Která pole (podle atributu `name`) přijímají placeholdery. */
  accept: (name: string) => boolean
  /** Zápis do formuláře (RHF `setValue` s `shouldDirty`). */
  setValue: (name: string, value: string) => void
  /** Pole, kam vkládat, když uživatel ještě do žádného neklikl. */
  fallback?: string
}

/**
 * Sleduje poslední zaměřené textové pole (`onFocusCapture` na `<form>`)
 * a vloží do něj token místo výběru. Kurzor skončí za vloženým tokenem.
 */
export function usePlaceholderInsert({ accept, setValue, fallback }: Options) {
  const target = useRef<TextEl | null>(null)
  const [activeName, setActiveName] = useState<string | null>(null)

  const onFocusCapture = (e: FocusEvent<HTMLElement>) => {
    const el = e.target
    if ((el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) && el.name && accept(el.name)) {
      target.current = el
      setActiveName(el.name)
    }
  }

  const insert = (token: string) => {
    let el = target.current
    if (!el?.isConnected && fallback) {
      el = document.querySelector<TextEl>(`[name="${CSS.escape(fallback)}"]`)
    }
    if (!el?.isConnected) return
    const { value, caret } = insertAt(el.value, token, el.selectionStart, el.selectionEnd)
    setValue(el.name, value)
    const node = el
    target.current = node
    setActiveName(node.name)
    requestAnimationFrame(() => {
      node.focus({ preventScroll: true })
      node.setSelectionRange(caret, caret)
    })
  }

  return { onFocusCapture, insert, activeName }
}
