// SPA adresy záznamů podle typu (události, úkoly). Typy odpovídají `subject_type` / `related_type` v API.

export type RecordKind =
  | 'invoice'
  | 'expense'
  | 'subject'
  | 'price_item'
  | 'recurring'
  | 'bank_transaction'
  | 'bank_account'
  | 'webhook'

/** `recordHref("acme", "invoice", 12)` → `"/a/acme/invoices/12"`; neznámý typ → `null`. */
export function recordHref(slug: string, type: string | undefined, id: number | undefined): string | null {
  if (!type) return null
  const base = `/a/${encodeURIComponent(slug)}`
  const withId = (path: string) => (id ? `${base}/${path}/${id}` : null)
  switch (type) {
    case 'invoice':
      return withId('invoices')
    case 'expense':
      return withId('expenses')
    case 'subject':
      return withId('subjects')
    case 'price_item':
      return withId('price-items')
    case 'recurring':
      return withId('recurring')
    case 'bank_transaction':
      return `${base}/bank`
    case 'bank_account':
      return `${base}/settings/bank-accounts`
    case 'webhook':
      return `${base}/settings/webhooks`
    default:
      return null
  }
}

export const recordKindLabels: Record<RecordKind, string> = {
  invoice: 'Faktura',
  expense: 'Náklad',
  subject: 'Kontakt',
  price_item: 'Položka ceníku',
  recurring: 'Pravidelná faktura',
  bank_transaction: 'Bankovní pohyb',
  bank_account: 'Bankovní účet',
  webhook: 'Webhook',
}
