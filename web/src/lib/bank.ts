// České bankovní účty a IBAN — zobrazovací/validační duplikát `internal/spayd`
// (zdrojem pravdy je backend, který vrací 422 při neplatném čísle).

export interface CzechAccount {
  prefix: string
  number: string
  bank: string
}

const ACCOUNT_RE = /^(?:(\d{1,6})-)?(\d{2,10})\/(\d{4})$/
const MOD11_WEIGHTS = [6, 3, 7, 9, 10, 5, 8, 4, 2, 1]

function mod11(part: string): boolean {
  const padded = part.padStart(10, '0')
  let sum = 0
  for (let i = 0; i < 10; i++) sum += Number(padded[i]) * MOD11_WEIGHTS[i]!
  return sum % 11 === 0
}

function compact(s: string): string {
  return s.trim().replace(/\s+/g, '')
}

export type CzechAccountError = 'format' | 'prefix' | 'number'

/** Rozebere „19-2000145399/0800“; vrací účet nebo druh chyby. */
export function checkCzechAccount(input: string): { account: CzechAccount } | { error: CzechAccountError } {
  const m = ACCOUNT_RE.exec(compact(input))
  if (!m) return { error: 'format' }
  const [, prefix = '', number = '', bank = ''] = m
  if (!mod11(prefix)) return { error: 'prefix' }
  if (!mod11(number)) return { error: 'number' }
  return { account: { prefix, number, bank } }
}

export function parseCzechAccount(input: string): CzechAccount | null {
  const r = checkCzechAccount(input)
  return 'account' in r ? r.account : null
}

export function czechAccountErrorMessage(error: CzechAccountError): string {
  switch (error) {
    case 'format':
      return 'Zadejte číslo ve tvaru předčíslí-číslo/kód banky, např. 19-2000145399/0800'
    case 'prefix':
      return 'Předčíslí nesplňuje kontrolu (modulo 11) — zkontrolujte překlep'
    case 'number':
      return 'Číslo účtu nesplňuje kontrolu (modulo 11) — zkontrolujte překlep'
  }
}

/** Zbytek po dělení 97 pro libovolně dlouhý řetězec číslic. */
function mod97(digits: string): number {
  let rem = 0
  for (const ch of digits) rem = (rem * 10 + Number(ch)) % 97
  return rem
}

/** IBAN českého účtu (CZkk BBBB PPPPPP NNNNNNNNNN). */
export function czechAccountToIban(a: CzechAccount): string {
  const bban = a.bank + a.prefix.padStart(6, '0') + a.number.padStart(10, '0')
  const check = 98 - mod97(bban + '123500') // „CZ“ = 12 35, kontrolní číslice 00
  return `CZ${String(check).padStart(2, '0')}${bban}`
}

export function normalizeIban(s: string): string {
  return s.replace(/\s+/g, '').toUpperCase()
}

/** Strukturální kontrola IBAN (ISO 13616, mod 97). */
export function isValidIban(input: string): boolean {
  const s = normalizeIban(input)
  if (s.length < 15 || s.length > 34 || !/^[A-Z]{2}\d{2}[A-Z0-9]+$/.test(s)) return false
  const rearranged = s.slice(4) + s.slice(0, 4)
  let digits = ''
  for (const ch of rearranged) {
    digits += /\d/.test(ch) ? ch : String(ch.charCodeAt(0) - 55)
  }
  return mod97(digits) === 1
}

/** „CZ6508000000192000145399“ → „CZ65 0800 0000 1920 0014 5399“. */
export function formatIban(input: string): string {
  return normalizeIban(input).replace(/(.{4})(?=.)/g, '$1 ')
}

const SWIFT_RE = /^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$/

export function isValidSwift(input: string): boolean {
  return SWIFT_RE.test(input.trim().toUpperCase())
}

interface BankInfo {
  name: string
  swift?: string
}

/** Nejčastější české banky (kód → název, BIC). */
const BANKS: Record<string, BankInfo> = {
  '0100': { name: 'Komerční banka', swift: 'KOMBCZPP' },
  '0300': { name: 'ČSOB', swift: 'CEKOCZPP' },
  '0600': { name: 'MONETA Money Bank', swift: 'AGBACZPP' },
  '0710': { name: 'Česká národní banka', swift: 'CNBACZPP' },
  '0800': { name: 'Česká spořitelna', swift: 'GIBACZPX' },
  '2010': { name: 'Fio banka', swift: 'FIOBCZPP' },
  '2060': { name: 'Citfin', swift: 'CITFCZPP' },
  '2700': { name: 'UniCredit Bank', swift: 'BACXCZPP' },
  '3030': { name: 'Air Bank', swift: 'AIRACZPP' },
  '3500': { name: 'ING Bank', swift: 'INGBCZPP' },
  '4300': { name: 'Národní rozvojová banka', swift: 'NROZCZPP' },
  '5500': { name: 'Raiffeisenbank', swift: 'RZBCCZPP' },
  '5800': { name: 'J&T Banka', swift: 'JTBPCZPP' },
  '6000': { name: 'PPF banka', swift: 'PMBPCZPP' },
  '6100': { name: 'Raiffeisenbank (Equa)', swift: 'EQBKCZPP' },
  '6210': { name: 'mBank', swift: 'BREXCZPP' },
  '7910': { name: 'Deutsche Bank', swift: 'DEUTCZPX' },
  '8040': { name: 'Oberbank', swift: 'OBKLCZ2X' },
  '8250': { name: 'Bank of China', swift: 'BKCHCZPP' },
  '2250': { name: 'Banka CREDITAS' },
  '2600': { name: 'Citibank Europe' },
  '6363': { name: 'Partners Banka' },
}

export function bankName(code: string): string | undefined {
  return BANKS[code]?.name
}

export function bankSwift(code: string): string | undefined {
  return BANKS[code]?.swift
}
