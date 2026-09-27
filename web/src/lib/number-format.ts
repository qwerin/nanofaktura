// Formáty číselných řad — zobrazovací duplikát `internal/numbering` (živý náhled při psaní).
// Placeholdery: {YYYY}, {YY}, {MM}, {N}…{NNNNNN} (počet N = doplnění nulami). Aspoň jedno {N…}.

export const MAX_FORMAT_LENGTH = 50

type Token = { kind: 'lit'; text: string } | { kind: 'YYYY' | 'YY' | 'MM' } | { kind: 'N'; width: number }

export type ParseResult = { ok: true; tokens: Token[] } | { ok: false; error: string }

export function parseNumberFormat(format: string): ParseResult {
  if (format === '') return { ok: false, error: 'Zadejte formát' }
  if (format.length > MAX_FORMAT_LENGTH) return { ok: false, error: `Nejvýše ${MAX_FORMAT_LENGTH} znaků` }
  const tokens: Token[] = []
  let hasN = false
  let rest = format
  while (rest !== '') {
    const open = rest.indexOf('{')
    const close = rest.indexOf('}')
    if (open < 0) {
      if (close >= 0) return { ok: false, error: 'Nadbytečná závorka „}“' }
      tokens.push({ kind: 'lit', text: rest })
      break
    }
    if (close >= 0 && close < open) return { ok: false, error: 'Nadbytečná závorka „}“' }
    if (open > 0) tokens.push({ kind: 'lit', text: rest.slice(0, open) })
    const end = rest.indexOf('}', open)
    if (end < 0) return { ok: false, error: 'Neuzavřená závorka „{“' }
    const name = rest.slice(open + 1, end)
    if (name === 'YYYY' || name === 'YY' || name === 'MM') {
      tokens.push({ kind: name })
    } else if (/^N{1,6}$/.test(name)) {
      tokens.push({ kind: 'N', width: name.length })
      hasN = true
    } else {
      return { ok: false, error: `Neznámý zástupný symbol {${name}}` }
    }
    rest = rest.slice(end + 1)
  }
  if (!hasN) return { ok: false, error: 'Formát musí obsahovat pořadové číslo {N}…{NNNNNN}' }
  return { ok: true, tokens }
}

/** Chybová hláška formátu, nebo `null` když je platný. */
export function numberFormatError(format: string): string | null {
  const r = parseNumberFormat(format)
  return r.ok ? null : r.error
}

/** Vykreslí číslo dokladu: renderNumberFormat('{YYYY}-{NNNN}', 2026-03-01, 7) = '2026-0007'. */
export function renderNumberFormat(format: string, date: Date, n: number): string | null {
  const r = parseNumberFormat(format)
  if (!r.ok) return null
  return r.tokens
    .map((t) => {
      switch (t.kind) {
        case 'lit':
          return t.text
        case 'YYYY':
          return String(date.getFullYear()).padStart(4, '0')
        case 'YY':
          return String(date.getFullYear() % 100).padStart(2, '0')
        case 'MM':
          return String(date.getMonth() + 1).padStart(2, '0')
        case 'N':
          return String(n).padStart(t.width, '0')
      }
    })
    .join('')
}

/** Jak často se řada restartuje (podle obsažených placeholderů). */
export function numberFormatPeriod(format: string): 'month' | 'year' | 'never' | null {
  const r = parseNumberFormat(format)
  if (!r.ok) return null
  if (r.tokens.some((t) => t.kind === 'MM')) return 'month'
  if (r.tokens.some((t) => t.kind === 'YYYY' || t.kind === 'YY')) return 'year'
  return 'never'
}

/** Vloží text na pozici kurzoru (nahradí výběr); vrací nový text a pozici kurzoru. */
export function insertAt(value: string, insert: string, start: number, end: number = start): { value: string; cursor: number } {
  const s = Math.max(0, Math.min(start, value.length))
  const e = Math.max(s, Math.min(end, value.length))
  return { value: value.slice(0, s) + insert + value.slice(e), cursor: s + insert.length }
}
