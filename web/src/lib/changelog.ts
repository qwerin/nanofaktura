// Novinky pro uživatele: parser CHANGELOG.md (kořen repa) do strukturovaných záznamů.
// Podporuje jen podmnožinu Markdownu popsanou v hlavičce souboru — žádné HTML se nevkládá, vše se renderuje jako text.

export type Inline =
  | { type: 'text'; text: string }
  | { type: 'strong'; text: string }
  | { type: 'em'; text: string }
  | { type: 'code'; text: string }
  | { type: 'link'; text: string; href: string }

export type Block =
  | { type: 'heading'; content: Inline[] }
  | { type: 'paragraph'; content: Inline[] }
  | { type: 'list'; items: Inline[][] }

export interface ChangelogEntry {
  /** Datum vydání `YYYY-MM-DD` — slouží i jako identifikátor pro „přečteno“. */
  date: string
  title: string
  blocks: Block[]
}

const ENTRY_HEADING = /^##\s+(\d{4}-\d{2}-\d{2})\s*(?:[·—–-]\s*(.*))?$/
const INLINE = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)|\*([^*\s][^*]*?)\*/g

export function parseInline(text: string): Inline[] {
  const out: Inline[] = []
  let last = 0
  for (const m of text.matchAll(INLINE)) {
    if (m.index > last) out.push({ type: 'text', text: text.slice(last, m.index) })
    if (m[1] !== undefined) out.push({ type: 'strong', text: m[1] })
    else if (m[2] !== undefined) out.push({ type: 'code', text: m[2] })
    else if (m[3] !== undefined) out.push({ type: 'link', text: m[3], href: m[4] })
    else out.push({ type: 'em', text: m[5] })
    last = m.index + m[0].length
  }
  if (last < text.length) out.push({ type: 'text', text: text.slice(last) })
  return out
}

export function parseChangelog(md: string): ChangelogEntry[] {
  const entries: ChangelogEntry[] = []
  const source = md.replace(/<!--[\s\S]*?-->/g, '')
  let entry: ChangelogEntry | undefined
  let paragraph: string[] = []

  const flushParagraph = () => {
    if (entry && paragraph.length) entry.blocks.push({ type: 'paragraph', content: parseInline(paragraph.join(' ')) })
    paragraph = []
  }

  for (const raw of source.split(/\r?\n/)) {
    const line = raw.trim()
    const heading = ENTRY_HEADING.exec(line)
    if (heading) {
      flushParagraph()
      entry = { date: heading[1], title: heading[2]?.trim() ?? '', blocks: [] }
      entries.push(entry)
      continue
    }
    if (!entry) continue // úvodní nadpis a text před prvním záznamem
    if (line === '') {
      flushParagraph()
    } else if (line.startsWith('### ')) {
      flushParagraph()
      entry.blocks.push({ type: 'heading', content: parseInline(line.slice(4)) })
    } else if (/^[-*]\s+/.test(line)) {
      flushParagraph()
      const item = parseInline(line.replace(/^[-*]\s+/, ''))
      const prev = entry.blocks.at(-1)
      if (prev?.type === 'list') prev.items.push(item)
      else entry.blocks.push({ type: 'list', items: [item] })
    } else {
      paragraph.push(line)
    }
  }
  flushParagraph()
  return entries.sort((a, b) => b.date.localeCompare(a.date))
}

/** Záznamy novější než naposledy přečtené datum (bez uloženého data = vše nové). */
export function unseenEntries(entries: ChangelogEntry[], lastSeen: string | null): ChangelogEntry[] {
  return lastSeen ? entries.filter((e) => e.date > lastSeen) : entries
}
