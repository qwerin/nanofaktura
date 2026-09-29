// Čisté pomocníky stránky DPH: řádky přiznání s popisky v češtině a překlad varování backendu.

import type { VatPair, VatReturn } from '@/api/types'

export interface VatReturnRow {
  /** Číslo řádku v přiznání (DPHDP3). */
  line: string
  label: string
  hint?: string
  base?: number
  vat?: number
  /** Řádek výsledku (ř. 62–65) — zvýrazněný. */
  total?: boolean
}

export interface VatReturnSection {
  title: string
  rows: VatReturnRow[]
}

const pair = (p: VatPair) => ({ base: p.base, vat: p.vat })

/** Sekce přiznání v pořadí formuláře. Nulové řádky mimo výsledky se vynechají (`hideEmpty`). */
export function vatReturnSections(r: VatReturn, { hideEmpty = false } = {}): VatReturnSection[] {
  const sections: VatReturnSection[] = [
    {
      title: 'Uskutečněná plnění (daň na výstupu)',
      rows: [
        { line: '1', label: 'Prodej v tuzemsku, základní sazba 21 %', ...pair(r.r1) },
        { line: '2', label: 'Prodej v tuzemsku, snížená sazba 12 %', ...pair(r.r2) },
      ],
    },
    {
      title: 'Plnění bez daně s nárokem na odpočet',
      rows: [
        {
          line: '21',
          label: 'Služby do jiného státu EU',
          hint: 'Daň odvádí zákazník (reverse charge, § 102).',
          base: r.r21,
        },
        {
          line: '25',
          label: 'Tuzemské přenesení daňové povinnosti — dodavatel',
          hint: 'Stavební práce, šrot apod. (§ 92a); daň přiznává odběratel.',
          base: r.r25,
        },
        { line: '26', label: 'Ostatní plnění s nárokem na odpočet', hint: 'Např. služby mimo EU.', base: r.r26 },
      ],
    },
    {
      title: 'Přijatá plnění (nárok na odpočet)',
      rows: [
        { line: '40', label: 'Nákupy od tuzemských plátců, základní sazba 21 %', ...pair(r.r40) },
        { line: '41', label: 'Nákupy od tuzemských plátců, snížená sazba 12 %', ...pair(r.r41) },
        { line: '46', label: 'Odpočet daně celkem', vat: r.r46 },
      ],
    },
    {
      title: 'Výsledek',
      rows: [
        { line: '62', label: 'Daň na výstupu celkem', vat: r.r62, total: true },
        { line: '63', label: 'Odpočet daně celkem', vat: r.r63, total: true },
        { line: '64', label: 'Vlastní daň — zaplatit', vat: r.r64, total: true },
        { line: '65', label: 'Nadměrný odpočet — vrátí finanční úřad', vat: r.r65, total: true },
      ],
    },
  ]
  if (!hideEmpty) return sections
  return sections
    .map((s) => ({ ...s, rows: s.rows.filter((row) => row.total || (row.base ?? 0) !== 0 || (row.vat ?? 0) !== 0) }))
    .filter((s) => s.rows.length > 0)
}

/** Výsledek přiznání: kolik zaplatit (kladné), nebo vrátit (záporné). */
export function vatBalance(r: VatReturn): number {
  return r.r64 > 0 ? r.r64 : -r.r65
}

const WARNING_PATTERNS: [RegExp, (m: RegExpExecArray) => string][] = [
  [
    /^(.+): VAT rate ([\d.]+) % is not reported \(only 21 % and 12 %\)$/,
    (m) => `${m[1]}: sazba ${m[2]!.replace(/\.?0+$/, '').replace('.', ',')} % se do přiznání nezahrnuje (jen 21 % a 12 %).`,
  ],
  [/^(.+): reverse charge without the customer's CZ DIČ$/, (m) => `${m[1]}: přenesená daňová povinnost, ale odběratel nemá české DIČ.`],
  [/^(.+): EU reverse charge without the customer's VAT number$/, (m) => `${m[1]}: služba do EU bez DIČ odběratele.`],
  [
    /^(.+): amount without VAT \((-?\d+)\.(\d{2}) Kč\) is not reported$/,
    (m) => `${m[1]}: částka s nulovou sazbou (${m[2]},${m[3]} Kč) se v přiznání neuvádí.`,
  ],
  [/^(.+): deduction skipped, the supplier has no CZ DIČ$/, (m) => `${m[1]}: odpočet vynechán — dodavatel nemá české DIČ (pořízení z EU / dovoz je potřeba doplnit ručně).`],
]

/** Varování backendu (anglicky) → česky; neznámý text vrátí beze změny. */
export function translateVatWarning(w: string): string {
  for (const [re, fmt] of WARNING_PATTERNS) {
    const m = re.exec(w)
    if (m) return fmt(m)
  }
  return w
}
