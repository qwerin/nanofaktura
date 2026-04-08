const MOD11_WEIGHTS = [6, 3, 7, 9, 10, 5, 8, 4, 2, 1]

function mod11valid(part: string): boolean {
  const padded = part.padStart(10, '0')
  let sum = 0
  for (let i = 0; i < 10; i++) sum += parseInt(padded[i]) * MOD11_WEIGHTS[i]
  return sum % 11 === 0
}

// Vrátí true pokud české číslo účtu "prefix-číslo/kód" nebo "číslo/kód" projde mod-11 kontrolou.
export function validateCzAccount(account: string): boolean {
  const m = account.trim().match(/^(?:(\d{1,6})-)?(\d{1,10})\/(\d{4})$/)
  if (!m) return false
  return mod11valid(m[1] ?? '') && mod11valid(m[2])
}

// Dopočítá CZ IBAN z českého čísla účtu "prefix-číslo/kód" nebo "číslo/kód".
// Vrátí prázdný řetězec pokud vstup není ve správném formátu.
export function czIban(account: string): string {
  const m = account.trim().match(/^(?:(\d{1,6})-)?(\d{1,10})\/(\d{4})$/)
  if (!m) return ''
  const prefix = (m[1] ?? '').padStart(6, '0')
  const num    = m[2].padStart(10, '0')
  const bank   = m[3]
  const bban   = bank + prefix + num          // 20 číslic
  // mod-97 přes string aritmetiku
  const digits = bban + '123500'              // CZ=1235, check=00
  let rem = 0
  for (const ch of digits) rem = (rem * 10 + parseInt(ch)) % 97
  const check = String(98 - rem).padStart(2, '0')
  return 'CZ' + check + bban
}

// Lookup tabulka kód banky → SWIFT/BIC pro české banky.
const CZ_BANK_SWIFT: Record<string, string> = {
  '0100': 'KOMBCZPP', // Komerční banka
  '0300': 'CEKOCZPP', // ČSOB
  '0600': 'AGBACZPP', // Moneta Money Bank
  '0710': 'CNBACZPP', // Česká národní banka
  '0800': 'GIBACZPX', // Česká spořitelna
  '2010': 'FIOBCZPP', // Fio banka
  '2060': 'CITFCZPP', // Citfin
  '2100': 'ERBBCZPP', // Expobank CZ
  '2700': 'BACXCZPP', // UniCredit Bank
  '3030': 'AIRACZPP', // Air Bank
  '3500': 'INGBCZPP', // ING Bank
  '4300': 'CMZBCZPP', // Moravský Peněžní Ústav
  '5500': 'RZBCCZPP', // Raiffeisenbank
  '5800': 'JTBPCZPP', // J&T Banka
  '6000': 'PMBPCZPP', // PPF banka
  '6100': 'EQBKCZPP', // Equa bank
  '6200': 'COBACZPX', // Commerzbank
  '6210': 'BREXCZPP', // mBank
  '7910': 'DEUTCZPX', // Deutsche Bank
  '8040': 'OBKLCZPP', // Oberbank
  '8240': 'BSAGCZPP', // Banka CREDITAS
  '8250': 'BKCHCZPP', // Bank of China
}

// Vrátí SWIFT/BIC pro český kód banky (4 číslice za lomítkem).
// Vrátí prázdný řetězec pokud kód není v tabulce.
export function czSwift(account: string): string {
  const m = account.trim().match(/\/(\d{4})$/)
  if (!m) return ''
  return CZ_BANK_SWIFT[m[1]] ?? ''
}
