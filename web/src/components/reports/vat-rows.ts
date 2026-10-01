// Čisté pomocníky stránky DPH: řádky přiznání s popisky v češtině a překlad varování backendu.

import type { VatPair, VatReturn, VatWarning } from '@/api/types'

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
      title: 'Daň přiznává příjemce (samovyměření)',
      rows: [
        { line: '3', label: 'Pořízení zboží z jiného členského státu, základní sazba 21 %', ...pair(r.r3) },
        { line: '4', label: 'Pořízení zboží z jiného členského státu, snížená sazba 12 %', ...pair(r.r4) },
        {
          line: '5',
          label: 'Přijetí služby od osoby registrované v jiném členském státě, základní sazba 21 %',
          ...pair(r.r5),
        },
        {
          line: '6',
          label: 'Přijetí služby od osoby registrované v jiném členském státě, snížená sazba 12 %',
          ...pair(r.r6),
        },
        {
          line: '10',
          label: 'Přenesení daňové povinnosti – příjemce, základní sazba 21 %',
          hint: 'Tuzemské přenesení (§ 92a), např. stavební práce.',
          ...pair(r.r10),
        },
        { line: '11', label: 'Přenesení daňové povinnosti – příjemce, snížená sazba 12 %', ...pair(r.r11) },
        {
          line: '12',
          label: 'Ostatní plnění s povinností přiznat daň příjemcem, základní sazba 21 %',
          hint: 'Např. služby ze zemí mimo EU.',
          ...pair(r.r12),
        },
        { line: '13', label: 'Ostatní plnění s povinností přiznat daň příjemcem, snížená sazba 12 %', ...pair(r.r13) },
      ],
    },
    {
      title: 'Plnění bez daně s nárokem na odpočet',
      rows: [
        {
          line: '20',
          label: 'Dodání zboží do jiného členského státu',
          hint: 'Osvobozeno (§ 64), zákazník má DIČ v jiném státě EU.',
          base: r.r20,
        },
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
        { line: '43', label: 'Odpočet z plnění ř. 3–13, základní sazba 21 %', ...pair(r.r43) },
        { line: '44', label: 'Odpočet z plnění ř. 3–13, snížená sazba 12 %', ...pair(r.r44) },
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

const WARNING_TEXTS: Record<VatWarning['code'], (p: Record<string, string>) => string> = {
  unsupported_rate: (p) =>
    `sazba ${(p.rate ?? '').replace(/\.?0+$/, '').replace('.', ',')} % se do přiznání nezahrnuje (jen 21 % a 12 %).`,
  reverse_charge_no_dic: () => 'přenesená daňová povinnost, ale odběratel nemá české DIČ.',
  eu_reverse_charge_no_vat: () => 'služba do EU bez DIČ odběratele.',
  zero_rate_not_reported: (p) => `částka s nulovou sazbou (${(p.amount ?? '').replace('.', ',')} Kč) se v přiznání neuvádí.`,
  supplier_no_dic: () => 'odpočet vynechán — dodavatel nemá české DIČ (pořízení z EU / dovoz je potřeba doplnit ručně).',
  calculation_error: () => 'doklad nejde přepočítat, zkontrolujte částky a kurz.',
  missing_taxable_date: () => 'chybí DUZP — v přiznání je podle data vystavení. Doplňte DUZP.',
  possible_reverse_charge: () =>
    'dodavatel neúčtoval DPH. Pokud daň přiznáváte vy (služby či zboží ze zahraničí, § 92a), označte náklad jako přenesenou daňovou povinnost.',
  reverse_charge_import: () => 'zboží ze zemí mimo EU je dovoz (DPH vyměří celní úřad) — do přiznání se neuvádí.',
  reverse_charge_subject_code: () => 'v kontrolním hlášení doplňte v EPO kód předmětu plnění (§ 92a).',
  ec_sales_list: () => 'plnění do jiných států EU (ř. 20 a 21) podejte také v souhrnném hlášení.',
  correction_of_cancelled: () => 'opravuje stornovanou fakturu, původní doklad v přiznání není.',
  control_statement_monthly: () =>
    'právnická osoba podává kontrolní hlášení měsíčně — vytvořte ho za každý měsíc čtvrtletí.',
}

/** Varování VAT reportu (kód + parametry) → česky; neznámý kód = anglická zpráva backendu. */
export function translateVatWarning(w: VatWarning): string {
  const fmt = WARNING_TEXTS[w.code] as ((p: Record<string, string>) => string) | undefined
  if (!fmt) return w.message
  const text = fmt(w.params ?? {})
  // Varování za celé období nemají doklad — věta pak začíná velkým písmenem.
  return w.document ? `${w.document}: ${text}` : text.charAt(0).toUpperCase() + text.slice(1)
}
