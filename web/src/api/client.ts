// Typovaný API klient generovaný ručně z OpenAPI schématu backendu.
// Komunikuje přes /api proxy (Vite dev) nebo relativní URL (produkce v single binary).

export interface InvoiceLine {
  id?: number
  invoice_id?: number
  position: number
  name: string
  quantity: string        // decimal string e.g. "1.5"
  unit_name: string
  unit_price_hal: number  // v haléřích
  vat_rate_bps: number    // basis points (2100 = 21 %)
  unit_price_without_vat_hal?: number
  unit_price_with_vat_hal?: number
  total_price_without_vat_hal?: number
  total_vat_hal?: number
  total_hal?: number
}

export type InvoiceStatus = 'open' | 'sent' | 'overdue' | 'paid' | 'cancelled'
export type DocumentType = 'invoice' | 'proforma' | 'correction' | 'tax_document'
export type PaymentMethod = 'bank' | 'cash' | 'card' | 'cod' | 'paypal'

export interface Invoice {
  id?: number
  document_type?: DocumentType
  status?: InvoiceStatus
  language?: string

  number: string
  variable_symbol?: string
  order_number?: string
  custom_id?: string

  // Dates as "YYYY-MM-DD" strings
  issued_on: string
  taxable_fulfillment_due: string
  due: number        // days until overdue
  paid_on?: string

  subject_id?: number

  // Your company info
  your_name?: string
  your_street?: string
  your_city?: string
  your_zip?: string
  your_country?: string
  your_registration_no?: string
  your_vat_no?: string

  // Client info
  client_name: string
  client_street?: string
  client_city?: string
  client_zip?: string
  client_country?: string
  client_registration_no?: string
  client_vat_no?: string

  // Delivery address
  client_has_delivery_address?: boolean
  client_delivery_name?: string
  client_delivery_street?: string
  client_delivery_city?: string
  client_delivery_zip?: string
  client_delivery_country?: string

  // Payment
  payment_method?: PaymentMethod
  bank_account?: string
  iban?: string
  swift_bic?: string

  // Currency
  currency?: string
  exchange_rate?: string

  // Tax
  vat_exempt?: boolean
  transferred_tax_liability?: boolean
  vat_price_mode?: string

  // Notes
  note?: string
  footer_note?: string
  private_note?: string
  tags?: string

  // Totals (computed by backend)
  subtotal?: number
  total_vat_hal?: number
  total?: number

  lines: InvoiceLine[]
}

export type UserRole = 'superadmin' | 'admin' | 'user'

export interface User {
  id?: number
  username: string
  email?: string
  role: UserRole
  is_active: boolean
  created_at?: string
}

export type SubjectType = 'customer' | 'supplier' | 'both'

export interface Subject {
  id?: number
  custom_id?: string
  type?: SubjectType

  name: string
  street?: string
  city?: string
  zip?: string
  country?: string
  registration_no?: string
  vat_no?: string
  local_vat_no?: string

  email?: string
  phone?: string
  website?: string

  bank_account?: string
  iban?: string

  note?: string

  default_payment_method?: PaymentMethod
  default_due?: number
}

export interface AresSubject {
  ic: string
  name: string
  dic: string
  street: string
  city: string
  zip: string
  country_code: string
}

export interface NumberFormat {
  id?: number
  settings_id?: number
  document_type: string
  label?: string
  pattern: string
  next_number: number
  padding_width?: number
}

export interface Settings {
  id?: number
  company_name?: string
  company_street?: string
  company_city?: string
  company_zip?: string
  company_country?: string
  registration_no?: string
  vat_no?: string
  vat_exempt?: boolean
  bank_account?: string
  iban?: string
  swift_bic?: string
  default_due?: number
  default_currency?: string
  default_payment_method?: string
  default_note?: string
  invoice_template?: string
  number_formats?: NumberFormat[]
}

const BASE = '/api'

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ title: res.statusText }))
    throw new Error(err.title ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  const json = await res.json()
  // Huma v2 přidává "$schema" jako první klíč u objektových odpovědí; stripneme ho.
  // Pole (list endpointy) vracíme přímo.
  if (Array.isArray(json)) return json as T
  const { $schema: _s, ...data } = json as Record<string, unknown>
  return data as T
}

export const api = {
  health: () => request<{ status: string; version: string; multi_user: boolean; initialized: boolean }>('GET', '/health'),

  auth: {
    login:  (credentials: { username: string; password: string }) => request<User>('POST', '/auth/login', credentials),
    logout: () => request<void>('POST', '/auth/logout'),
    me:     () => request<User>('GET', '/auth/me'),
    setup:  (credentials: { username: string; password: string }) => request<User>('POST', '/setup/init', credentials),
  },

  users: {
    list:          ()                                                          => request<User[]>('GET',    '/users'),
    get:           (id: number)                                                => request<User>  ('GET',    `/users/${id}`),
    create:        (u: { username: string; password: string; email?: string; role?: UserRole }) => request<User>('POST', '/users', u),
    update:        (id: number, u: { email?: string; role?: UserRole; is_active?: boolean })    => request<User>('PUT',  `/users/${id}`, u),
    delete:        (id: number)                                                => request<void>  ('DELETE', `/users/${id}`),
    resetPassword: (id: number, password: string)                             => request<void>  ('POST',   `/users/${id}/reset-password`, { password }),
  },

  ares: {
    lookup: (ic: string) => request<AresSubject>('GET', `/ares/${ic}`),
  },

  invoices: {
    list:      ()                         => request<Invoice[]>('GET',  '/invoices'),
    get:       (id: number)               => request<Invoice>  ('GET',  `/invoices/${id}`),
    create:    (inv: Invoice)             => request<Invoice>  ('POST', '/invoices', inv),
    update:    (id: number, inv: Invoice) => request<Invoice>  ('PUT',  `/invoices/${id}`, inv),
    duplicate: (id: number)               => request<Invoice>  ('POST', `/invoices/${id}/duplicate`),
  },

  subjects: {
    list:   ()                         => request<Subject[]>('GET',    '/subjects'),
    get:    (id: number)               => request<Subject>  ('GET',    `/subjects/${id}`),
    create: (s: Subject)               => request<Subject>  ('POST',   '/subjects', s),
    update: (id: number, s: Subject)   => request<Subject>  ('PUT',    `/subjects/${id}`, s),
    delete: (id: number)               => request<void>     ('DELETE', `/subjects/${id}`),
  },

  settings: {
    get:    ()                              => request<Settings>      ('GET',    '/settings'),
    update: (s: Partial<Settings>)          => request<Settings>      ('PUT',    '/settings', s),
    formats: {
      list:   ()                            => request<NumberFormat[]>('GET',    '/settings/number-formats'),
      create: (f: NumberFormat)             => request<NumberFormat>  ('POST',   '/settings/number-formats', f),
      update: (id: number, f: NumberFormat) => request<NumberFormat>  ('PUT',    `/settings/number-formats/${id}`, f),
      delete: (id: number)                  => request<void>          ('DELETE', `/settings/number-formats/${id}`),
      next:   (id: number)                  => request<{number: string}>('POST', `/settings/number-formats/${id}/next`),
    },
  },
}
