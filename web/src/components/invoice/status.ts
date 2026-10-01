// Stavy faktur, popisky a povolené akce — zrcadlo stavového automatu backendu
// (internal/billing/status.go). Backend je zdrojem pravdy; tady jde jen o to,
// co v UI nabídnout.

import type { DocumentType, InvoiceAction, InvoiceStatus, PaymentMethod } from '@/api/types'
import { daysBetween, formatDate, todayISO } from '@/lib/date'

export const statusLabels: Record<InvoiceStatus, string> = {
  open: 'Otevřená',
  sent: 'Odeslaná',
  overdue: 'Po splatnosti',
  paid: 'Uhrazená',
  cancelled: 'Stornovaná',
  uncollectible: 'Nedobytná',
}

export const documentTypeLabels: Record<DocumentType, string> = {
  invoice: 'Faktura',
  proforma: 'Zálohová faktura',
  correction: 'Opravný doklad',
  tax_document: 'Daňový doklad k přijaté platbě',
}

/** Krátké popisky pro přepínače a filtry. */
export const documentTypeShortLabels: Record<DocumentType, string> = {
  invoice: 'Faktura',
  proforma: 'Zálohová',
  correction: 'Opravný',
  tax_document: 'Daňový doklad k platbě',
}

export const paymentMethodLabels: Record<PaymentMethod, string> = {
  bank: 'Bankovní převod',
  cash: 'Hotově',
  card: 'Kartou',
  cod: 'Dobírka',
  paypal: 'PayPal',
  custom: 'Jiný',
}

/** České tvary podle počtu: `plural(3, ['den', 'dny', 'dní'])` → „dny“. */
export function plural(n: number, forms: readonly [one: string, few: string, many: string]): string {
  const abs = Math.abs(n)
  if (abs === 1) return forms[0]
  if (abs >= 2 && abs <= 4) return forms[1]
  return forms[2]
}

export function daysText(n: number): string {
  return `${n} ${plural(n, ['den', 'dny', 'dní'])}`
}

export type DueTone = 'muted' | 'warning' | 'destructive' | 'success'

/** Tailwind třídy textu podle naléhavosti splatnosti. */
export const dueToneClass: Record<DueTone, string> = {
  muted: 'text-muted-foreground',
  warning: 'text-warning-foreground dark:text-warning',
  destructive: 'text-destructive',
  success: 'text-success',
}

interface DueSource {
  status: InvoiceStatus
  due_on: string
  paid_on: string
}

/**
 * Text o splatnosti do seznamu: „splatná za 3 dny“, „splatná dnes“, „5 dní po splatnosti“,
 * „uhrazena 3. 9. 2026“. Pro stornované a nedobytné vrací `null`.
 */
export function dueInfo(inv: DueSource, today: string = todayISO()): { text: string; tone: DueTone } | null {
  if (inv.status === 'paid') return { text: inv.paid_on ? `uhrazena ${formatDate(inv.paid_on)}` : 'uhrazena', tone: 'success' }
  if (inv.status === 'cancelled' || inv.status === 'uncollectible') return null
  if (!inv.due_on) return null
  const diff = daysBetween(today, inv.due_on)
  if (diff < 0) return { text: `${daysText(-diff)} po splatnosti`, tone: 'destructive' }
  if (diff === 0) return { text: 'splatná dnes', tone: 'warning' }
  if (diff === 1) return { text: 'splatná zítra', tone: 'warning' }
  return { text: `splatná za ${daysText(diff)}`, tone: diff <= 3 ? 'warning' : 'muted' }
}

interface ActionSource {
  status: InvoiceStatus
  document_type: DocumentType
  sent_at?: string
  locked_at?: string
  paid_amount: number
  remaining_amount: number
  /** Počet plateb (u detailu `payments.length`). */
  payment_count: number
  /** Související doklady (detail) — u proformy odhalí vyúčtovací fakturu. */
  related_documents?: { document_type: DocumentType }[]
}

/** Proforma, ke které už existuje vyúčtovací faktura (je vyúčtovaná, i když zbývá částka). */
export function isSettledProforma(inv: Pick<ActionSource, 'document_type' | 'related_documents'>): boolean {
  return inv.document_type === 'proforma' && (inv.related_documents ?? []).some((d) => d.document_type === 'invoice')
}

/** Uložený stav — `overdue` je odvozený z `open`/`sent`. */
export function storedStatus(inv: Pick<ActionSource, 'status' | 'sent_at'>): Exclude<InvoiceStatus, 'overdue'> {
  if (inv.status === 'overdue') return inv.sent_at ? 'sent' : 'open'
  return inv.status
}

export type UiAction =
  | InvoiceAction
  | 'edit'
  | 'delete'
  | 'duplicate'
  | 'correction'
  | 'add_payment'
  | 'final_invoice'

/** Akce, které má smysl v daném stavu nabídnout (SPEC §4.5). */
export function allowedActions(inv: ActionSource): Set<UiAction> {
  const s = storedStatus(inv)
  const locked = Boolean(inv.locked_at)
  const live = s === 'open' || s === 'sent'
  const out = new Set<UiAction>()

  // Daňový doklad k přijaté platbě vzniká a zaniká s platbou zálohy — ručně se nemění.
  if (inv.document_type === 'tax_document') {
    if (s === 'open') out.add('mark_as_sent')
    out.add(locked ? 'unlock' : 'lock')
    return out
  }
  const settled = isSettledProforma(inv)

  out.add('duplicate')
  if (!locked && !settled && s !== 'cancelled' && s !== 'uncollectible') out.add('edit')
  if (!locked && inv.payment_count === 0) out.add('delete')
  if (s === 'open') out.add('mark_as_sent')
  if (live && inv.payment_count === 0) out.add('cancel')
  if (live) out.add('mark_as_uncollectible')
  if (s === 'cancelled') out.add('undo_cancel')
  if (s === 'uncollectible') out.add('undo_uncollectible')
  out.add(locked ? 'unlock' : 'lock')
  if (inv.document_type === 'invoice') out.add('correction')
  if (!settled && s !== 'cancelled' && s !== 'uncollectible' && inv.remaining_amount !== 0) out.add('add_payment')
  if (inv.document_type === 'proforma' && !settled && s !== 'cancelled' && s !== 'uncollectible') out.add('final_invoice')
  return out
}
