import { describe, expect, it } from 'vitest'
import changelogSource from '../../../CHANGELOG.md?raw'
import { parseChangelog, parseInline, unseenEntries } from './changelog'

describe('parseInline', () => {
  it('rozpozná tučné, kód a odkaz', () => {
    expect(parseInline('Stiskněte `Ctrl K`, **hledání** a [návod](https://example.cz/x).')).toEqual([
      { type: 'text', text: 'Stiskněte ' },
      { type: 'code', text: 'Ctrl K' },
      { type: 'text', text: ', ' },
      { type: 'strong', text: 'hledání' },
      { type: 'text', text: ' a ' },
      { type: 'link', text: 'návod', href: 'https://example.cz/x' },
      { type: 'text', text: '.' },
    ])
  })

  it('kurzíva jednou hvězdičkou', () => {
    expect(parseInline('v *Nastavení → Tým* a **x**')).toEqual([
      { type: 'text', text: 'v ' },
      { type: 'em', text: 'Nastavení → Tým' },
      { type: 'text', text: ' a ' },
      { type: 'strong', text: 'x' },
    ])
  })

  it('odkaz jinam než na http(s) nechá jako text', () => {
    expect(parseInline('[klik](javascript:alert(1))')).toEqual([{ type: 'text', text: '[klik](javascript:alert(1))' }])
  })
})

describe('parseChangelog', () => {
  const md = `# Novinky
<!-- komentář
## 2000-01-01 · schovaný -->
Úvod se ignoruje.

## 2026-01-02 · Starší

Odstavec
na dva řádky.

## 2026-03-04 · Novější
### Faktury
- **A** — první
- druhý
Poznámka`

  it('seřadí záznamy od nejnovějšího a rozdělí bloky', () => {
    const entries = parseChangelog(md)
    expect(entries.map((e) => [e.date, e.title])).toEqual([
      ['2026-03-04', 'Novější'],
      ['2026-01-02', 'Starší'],
    ])
    expect(entries[0].blocks.map((b) => b.type)).toEqual(['heading', 'list', 'paragraph'])
    expect(entries[0].blocks[1]).toMatchObject({ type: 'list', items: [[{ type: 'strong', text: 'A' }, { type: 'text' }], [{ text: 'druhý' }]] })
    expect(entries[1].blocks).toEqual([{ type: 'paragraph', content: [{ type: 'text', text: 'Odstavec na dva řádky.' }] }])
  })

  it('unseenEntries vrátí jen novější než přečtené', () => {
    const entries = parseChangelog(md)
    expect(unseenEntries(entries, null)).toHaveLength(2)
    expect(unseenEntries(entries, '2026-01-02').map((e) => e.date)).toEqual(['2026-03-04'])
    expect(unseenEntries(entries, '2026-03-04')).toHaveLength(0)
  })
})

describe('CHANGELOG.md v repozitáři', () => {
  const entries = parseChangelog(changelogSource)

  it('má platné záznamy: datum, název, obsah, unikátní data', () => {
    expect(entries.length).toBeGreaterThan(0)
    for (const e of entries) {
      expect(e.date).toMatch(/^\d{4}-\d{2}-\d{2}$/)
      expect(e.title, `záznam ${e.date} nemá název`).not.toBe('')
      expect(e.blocks.length, `záznam ${e.date} je prázdný`).toBeGreaterThan(0)
    }
    expect(new Set(entries.map((e) => e.date)).size).toBe(entries.length)
  })
})
