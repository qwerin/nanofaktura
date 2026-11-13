// EET (evidence tržeb) — čistá logika pro nastavení, dialog platby a detail faktury.
// Zdrojem pravdy je backend (validace, výchozí hodnoty); tady jen popisky a pohodlí formulářů.

import type { EETSale, EETSaleStatus } from '@/api/types'

export type EetTone = 'success' | 'warning' | 'destructive' | 'muted'

export const EET_STATUSES: readonly EETSaleStatus[] = ['pending', 'failed', 'confirmed', 'cancelled']

export const eetStatusLabels: Record<EETSaleStatus, string> = {
  confirmed: 'Zaevidováno',
  pending: 'Čeká na odeslání',
  failed: 'Odmítnuto',
  cancelled: 'Zrušeno',
}

export const eetStatusTones: Record<EETSaleStatus, EetTone> = {
  confirmed: 'success',
  pending: 'warning',
  failed: 'destructive',
  cancelled: 'muted',
}

/** Storno = záznam rušící jinou tržbu (záporná částka). */
export function isCancellation(sale: Pick<EETSale, 'cancels_id' | 'amount'>): boolean {
  return sale.cancels_id != null || sale.amount < 0
}

/** Znovu odeslat jde jen čekající nebo odmítnutou tržbu (jinak 409 `eet_not_retryable`). */
export function canRetrySale(sale: Pick<EETSale, 'status'>): boolean {
  return sale.status === 'pending' || sale.status === 'failed'
}

/** Smazání platby s touto tržbou pošle do EET storno. */
export function deleteSendsCancellation(sale: Pick<EETSale, 'status'> | undefined): boolean {
  return sale?.status === 'confirmed' || sale?.status === 'pending'
}

/** Uplynula zákonná lhůta 48 hodin a tržba stále není zaevidovaná? */
export function isPastDeadline(sale: Pick<EETSale, 'status' | 'deadline_at'>, now: Date = new Date()): boolean {
  if (!canRetrySale(sale)) return false
  const deadline = Date.parse(sale.deadline_at)
  return Number.isFinite(deadline) && deadline < now.getTime()
}

/**
 * Předvolba „kontaktní platba“ v dialogu platby: hotově a kartou se platí zpravidla na místě.
 * Rozhoduje skutečný způsob úhrady, proto jde volbu vždy změnit.
 */
export function defaultContactPayment(paymentMethod: string): boolean {
  return paymentMethod === 'cash' || paymentMethod === 'card'
}

export interface InvoiceSales<S> {
  /** Tržba platby (podle `payment_id`). */
  byPayment: Map<number, S>
  /** Storna a tržby, jejichž platba už neexistuje — aby nic nezůstalo skryté. */
  other: S[]
}

/** Rozdělí tržby faktury na ty u plateb a ostatní (storna, tržby smazaných plateb). */
export function splitInvoiceSales<S extends Pick<EETSale, 'id' | 'payment_id' | 'cancels_id' | 'amount'>>(
  payments: readonly { id: number }[],
  sales: readonly S[],
): InvoiceSales<S> {
  const ids = new Set(payments.map((p) => p.id))
  const byPayment = new Map<number, S>()
  const other: S[] = []
  for (const s of sales) {
    if (s.payment_id == null || !ids.has(s.payment_id) || isCancellation(s)) {
      other.push(s)
      continue
    }
    const prev = byPayment.get(s.payment_id)
    if (!prev) {
      byPayment.set(s.payment_id, s)
    } else if (s.id > prev.id) {
      // U platby ukazujeme nejnovější záznam, starší jde do seznamu.
      byPayment.set(s.payment_id, s)
      other.push(prev)
    } else {
      other.push(s)
    }
  }
  return { byPayment, other }
}

// --- Údaje provozovny (stejná pravidla jako backend, který vrací 422) ---

const EIC_RE = /^CZ\d{8,10}$/
const UNIT_RE = /^[1-9]\d{0,8}$/
const REGISTER_RE = /^[A-Za-z0-9 .,:;/#\-_]{1,20}$/

export function eicError(v: string): string | null {
  return EIC_RE.test(v) ? null : 'EIČ má tvar CZ a 8–10 číslic (obvykle vaše DIČ)'
}

export function unitIdError(v: string): string | null {
  return UNIT_RE.test(v) ? null : 'Číslo evidenční jednotky je 1–9 číslic a nezačíná nulou'
}

export function registerIdError(v: string): string | null {
  return REGISTER_RE.test(v) ? null : 'Nejvýše 20 znaků: písmena bez diakritiky, číslice, mezera a . , : ; / # - _'
}

/** Návrh EIČ z DIČ účtu — jen když EIČ ještě není vyplněné a DIČ má český tvar. */
export function suggestEic(vatNo: string | null | undefined, eic: string): string | null {
  if (eic.trim() !== '') return null
  const candidate = (vatNo ?? '').replace(/\s+/g, '').toUpperCase()
  return EIC_RE.test(candidate) ? candidate : null
}

// --- Certifikát ---

export const CERTIFICATE_WARN_DAYS = 30
const DAY_MS = 24 * 60 * 60 * 1000

export type CertificateWarning = { level: 'expired' } | { level: 'expiring'; days: number }

/** Varování k platnosti certifikátu: už vypršel, nebo vyprší do 30 dnů. */
export function certificateWarning(validTo: string | null | undefined, now: Date = new Date()): CertificateWarning | null {
  if (!validTo) return null
  const end = Date.parse(validTo)
  if (!Number.isFinite(end)) return null
  const left = end - now.getTime()
  if (left <= 0) return { level: 'expired' }
  const days = Math.floor(left / DAY_MS)
  return days < CERTIFICATE_WARN_DAYS ? { level: 'expiring', days } : null
}

export function certificateWarningText(w: CertificateWarning): string {
  if (w.level === 'expired') return 'Platnost certifikátu vypršela — tržby nejde evidovat. Stáhněte si v portálu MOJE daně nový a nahrajte ho.'
  const when = w.days === 0 ? 'dnes' : w.days === 1 ? 'zítra' : `za ${w.days} ${plural(w.days, 'den', 'dny', 'dní')}`
  return `Platnost certifikátu vyprší ${when}. Stáhněte si v portálu MOJE daně nový a nahrajte ho včas.`
}

/** Certifikát vystavený na jiné EIČ, než je v nastavení (porovnání bez ohledu na velikost písmen). */
export function certificateEicMismatch(certificateEic: string | null | undefined, eic: string): boolean {
  if (!certificateEic || !eic.trim()) return false
  return certificateEic.trim().toUpperCase() !== eic.trim().toUpperCase()
}

/** Bajty → base64 (po částech, aby velký soubor nepřetekl zásobník volání). */
export function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const CHUNK = 0x8000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(binary)
}

/** Obsah souboru jako base64 (soubor .p12 se čte v prohlížeči a posílá v JSONu). */
export async function fileToBase64(file: Blob): Promise<string> {
  if (typeof file.arrayBuffer === 'function') return bytesToBase64(new Uint8Array(await file.arrayBuffer()))
  // Starší prohlížeče bez Blob.arrayBuffer().
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error ?? new Error('Soubor se nepodařilo přečíst'))
    reader.onload = () => resolve(bytesToBase64(new Uint8Array(reader.result as ArrayBuffer)))
    reader.readAsArrayBuffer(file)
  })
}

// --- Texty ---

function plural(n: number, one: string, few: string, many: string): string {
  const abs = Math.abs(n)
  return abs === 1 ? one : abs >= 2 && abs <= 4 ? few : many
}

/** „1 tržba“, „3 tržby“, „5 tržeb“. */
export function salesCountText(n: number): string {
  return `${n} ${plural(n, 'tržba', 'tržby', 'tržeb')}`
}

/** Věta do přehledu: kolik tržeb čeká na odeslání. */
export function pendingSummary(n: number): string {
  return `${salesCountText(n)} ${plural(n, 'čeká', 'čekají', 'čeká')} na odeslání — aplikace to zkouší sama znovu.`
}

/** Věta do přehledu: kolik tržeb Finanční správa odmítla. */
export function failedSummary(n: number): string {
  return `${n} ${plural(n, 'tržbu', 'tržby', 'tržeb')} Finanční správa odmítla — opravte příčinu a odešlete ${n === 1 ? 'ji' : 'je'} znovu.`
}
