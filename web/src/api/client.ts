// API klient pro NanoFaktura.
// TYPY: generovány z OpenAPI schématu backendu — needitovat ručně.
// Pro přegenerování spusť: make gen-types

import type { components } from './schema.gen'

// ── Response typy (full model, pro zobrazení/čtení) ───────────────────────────
export type Invoice        = components['schemas']['Invoice']
export type InvoiceLine    = components['schemas']['InvoiceLine']
export type Subject        = components['schemas']['Subject']
export type PriceItem      = components['schemas']['PriceItemOut']
export type StockMovement  = components['schemas']['StockMovement']
export type Settings       = components['schemas']['Settings']
export type NumberFormat   = components['schemas']['NumberFormat']
export type User           = components['schemas']['User']
export type AresSubject    = components['schemas']['Lookup']

// ── Input typy (DTO pro zápis, bez GORM/readonly polí) ───────────────────────
export type InvoiceInput       = components['schemas']['InvoiceInput']
export type LineInput          = components['schemas']['LineInput']
export type SubjectInput       = components['schemas']['SubjectInput']
export type PriceItemInput     = components['schemas']['PriceItemInput']
export type StockMovementInput = components['schemas']['StockMovementInput']
export type SettingsInput      = components['schemas']['SettingsInput']
export type NumberFormatInput  = components['schemas']['NumberFormatInput']

// ── Union typy (schema generuje 'string', zde zpřesňujeme) ───────────────────
export type PaymentMethod = 'bank' | 'cash' | 'card' | 'cod' | 'paypal'
export type InvoiceStatus = 'open' | 'overdue' | 'paid'
export type DocumentType  = 'invoice' | 'proforma' | 'correction' | 'tax_document'
export type SubjectType   = 'customer' | 'supplier' | 'both'
export type UserRole      = 'superadmin' | 'admin' | 'user'

const BASE = '/api'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (res.status === 401) {
    const returnTo = window.location.pathname + window.location.search
    window.location.href = '/login?returnTo=' + encodeURIComponent(returnTo)
    return undefined as T
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ title: res.statusText }))
    throw new Error(err.detail ?? err.title ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  const json = await res.json()
  // Huma v2 přidává "$schema" jako první klíč — stripneme ho.
  if (Array.isArray(json)) return json as T
  const { $schema: _s, ...data } = json as Record<string, unknown>
  return data as T
}

export const api = {
  health: () => request<components['schemas']['HealthOutputBody']>('GET', '/health'),

  auth: {
    login:  (credentials: { username: string; password: string }) => request<User>('POST', '/auth/login', credentials),
    logout: () => request<void>('POST', '/auth/logout'),
    me:     () => request<User>('GET', '/auth/me'),
    setup:  (credentials: { username: string; password: string }) => request<User>('POST', '/setup/init', credentials),
  },

  users: {
    list:          ()                                                                   => request<User[]>('GET',    '/users'),
    get:           (id: number)                                                         => request<User>  ('GET',    `/users/${id}`),
    create:        (u: components['schemas']['CreateUserInputBody'])                    => request<User>  ('POST',   '/users', u),
    update:        (id: number, u: components['schemas']['UpdateUserInputBody'])        => request<User>  ('PUT',    `/users/${id}`, u),
    delete:        (id: number)                                                         => request<void>  ('DELETE', `/users/${id}`),
    resetPassword: (id: number, password: string)                                      => request<void>  ('POST',   `/users/${id}/reset-password`, { password }),
  },

  ares: {
    lookup: (ic: string) => request<AresSubject>('GET', `/ares/${ic}`),
  },

  invoices: {
    list:      ()                                  => request<Invoice[]>('GET',  '/invoices'),
    get:       (id: number)                        => request<Invoice>  ('GET',  `/invoices/${id}`),
    create:    (inv: InvoiceInput)                 => request<Invoice>  ('POST', '/invoices', inv),
    update:    (id: number, inv: InvoiceInput)     => request<Invoice>  ('PUT',  `/invoices/${id}`, inv),
    duplicate: (id: number)                        => request<Invoice>  ('POST', `/invoices/${id}/duplicate`),
  },

  subjects: {
    list:   ()                            => request<Subject[]>('GET',    '/subjects'),
    get:    (id: number)                  => request<Subject>  ('GET',    `/subjects/${id}`),
    create: (s: SubjectInput)             => request<Subject>  ('POST',   '/subjects', s),
    update: (id: number, s: SubjectInput) => request<Subject>  ('PUT',    `/subjects/${id}`, s),
    delete: (id: number)                  => request<void>     ('DELETE', `/subjects/${id}`),
  },

  settings: {
    get:    ()                                     => request<Settings>      ('GET', '/settings'),
    update: (s: SettingsInput)                     => request<Settings>      ('PUT', '/settings', s),
    formats: {
      list:   ()                                   => request<NumberFormat[]>('GET',    '/settings/number-formats'),
      create: (f: NumberFormatInput)               => request<NumberFormat>  ('POST',   '/settings/number-formats', f),
      update: (id: number, f: NumberFormatInput)   => request<NumberFormat>  ('PUT',    `/settings/number-formats/${id}`, f),
      delete: (id: number)                         => request<void>          ('DELETE', `/settings/number-formats/${id}`),
      next:   (id: number)                         => request<{ number: string }>('POST', `/settings/number-formats/${id}/next`),
    },
  },

  priceItems: {
    list:    (archived = false)                        => request<PriceItem[]>    ('GET',    `/price-items${archived ? '?archived=true' : ''}`),
    get:     (id: number)                              => request<PriceItem>      ('GET',    `/price-items/${id}`),
    create:  (item: PriceItemInput)                    => request<PriceItem>      ('POST',   '/price-items', item),
    update:  (id: number, item: PriceItemInput)        => request<PriceItem>      ('PUT',    `/price-items/${id}`, item),
    archive: (id: number)                              => request<void>           ('DELETE', `/price-items/${id}`),
    movements: {
      list:   (itemId: number)                         => request<StockMovement[]>('GET',  `/price-items/${itemId}/movements`),
      create: (itemId: number, m: StockMovementInput)  => request<StockMovement> ('POST', `/price-items/${itemId}/movements`, m),
    },
  },
}
