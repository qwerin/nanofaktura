// Typované testovací fixtury odpovídající src/api/schema.gen.ts (komponentové testy).
import type { Account, AccountMembership, BankAccount, Invoice, Me, Subject } from '@/api/types'

export const SLUG = 'firma'
export const TS = '2026-03-01T10:00:00Z'

type Role = AccountMembership['role']

export function capabilitiesFor(role: Role): Account['capabilities'] {
  const manager = role === 'owner' || role === 'admin'
  return {
    edit: role !== 'accountant',
    export: role !== 'member',
    manage_members: manager,
    manage_owners: role === 'owner',
    manage_settings: manager,
    view_reports: role !== 'member',
  }
}

export function makeMe(overrides: Partial<Me> = {}, role: Role = 'owner'): Me {
  return {
    user: { id: 1, email: 'jan@example.cz', name: 'Jan Novák' },
    accounts: [{ slug: SLUG, name: 'Firma s.r.o.', role }],
    email_verified: true,
    instance_admin: false,
    instance_admin_pending: false,
    ...overrides,
  }
}

export function makeAccount(overrides: Partial<Account> = {}): Account {
  const role = overrides.role ?? 'owner'
  return {
    slug: SLUG,
    name: 'Firma s.r.o.',
    role,
    capabilities: capabilitiesFor(role),
    c_pracufo: '',
    c_ufo: '',
    street: 'Hlavní 1',
    city: 'Praha',
    zip: '11000',
    country: 'CZ',
    registration_no: '12345678',
    vat_no: '',
    vat_mode: 'non_vat_payer',
    vat_period: 'month',
    email: 'firma@example.cz',
    email_reply_to: '',
    email_signature: '',
    email_templates: [],
    phone: '',
    web: '',
    registered_by: '',
    default_currency: 'CZK',
    default_due_days: 14,
    default_footer_note: '',
    default_language: 'cs',
    default_note: '',
    default_payment_method: 'bank',
    default_vat_rate_bps: 2100,
    paid_thanks_enabled: false,
    pdf_accent: '#4f46e5',
    pdf_footer: '',
    pdf_show_qr: true,
    pdf_template: 'classic',
    reminder_days_after_due: [],
    reminders_enabled: false,
    round_total: false,
    onboarded_at: TS,
    created_at: TS,
    updated_at: TS,
    ...overrides,
  }
}

/** Prázdná stránkovaná odpověď seznamu. */
export function emptyList() {
  return { items: [], total: 0, page: 1, per_page: 50 }
}

export function makeSubject(overrides: Partial<Subject> = {}): Subject {
  return {
    id: 7,
    name: 'ACME a.s.',
    full_name: '',
    type: 'customer',
    registration_no: '87654321',
    vat_no: 'CZ87654321',
    local_vat_no: '',
    street: 'Dlouhá 5',
    city: 'Brno',
    zip: '60200',
    country: 'CZ',
    email: 'acme@example.cz',
    email_copy: '',
    phone: '',
    web: '',
    note: '',
    bank_account: '',
    iban: '',
    swift_bic: '',
    custom_id: null,
    due_days: null,
    created_at: TS,
    updated_at: TS,
    ...overrides,
  }
}

export function makeBankAccount(overrides: Partial<BankAccount> = {}): BankAccount {
  return {
    id: 3,
    name: 'Hlavní účet',
    number: '123456789/0800',
    iban: 'CZ6508000000192000145399',
    swift_bic: 'GIBACZPX',
    currency: 'CZK',
    is_default: true,
    has_fio_token: false,
    sync_provider: 'none',
    sync_from: '',
    created_at: TS,
    updated_at: TS,
    ...overrides,
  }
}

export function makeInvoice(overrides: Partial<Invoice> = {}): Invoice {
  return {
    id: 42,
    number: '2026-0001',
    document_type: 'invoice',
    status: 'sent',
    subject_id: 7,
    client_name: 'ACME a.s.',
    client_full_name: '',
    client_street: 'Dlouhá 5',
    client_city: 'Brno',
    client_zip: '60200',
    client_country: 'CZ',
    client_email: 'acme@example.cz',
    client_registration_no: '87654321',
    client_vat_no: 'CZ87654321',
    your_name: 'Firma s.r.o.',
    your_street: 'Hlavní 1',
    your_city: 'Praha',
    your_zip: '11000',
    your_country: 'CZ',
    your_registration_no: '12345678',
    your_vat_no: '',
    your_vat_mode: 'non_vat_payer',
    your_registered_by: '',
    issued_on: '2026-03-01',
    taxable_fulfillment_due: '2026-03-01',
    due_days: 14,
    due_on: '2026-03-15',
    currency: 'CZK',
    exchange_rate: '1',
    language: 'cs',
    payment_method: 'bank',
    custom_payment_method: '',
    bank_account: '123456789/0800',
    bank_account_id: 3,
    iban: 'CZ6508000000192000145399',
    swift_bic: 'GIBACZPX',
    variable_symbol: '20260001',
    order_number: '',
    note: '',
    footer_note: '',
    private_note: '',
    tags: [],
    prices_include_vat: false,
    reverse_charge: false,
    round_total: false,
    rounding: 0,
    subtotal: 1000000,
    vat_total: 0,
    total: 1000000,
    paid_amount: 0,
    paid_on: '',
    remaining_amount: 1000000,
    lines: [
      {
        id: 1,
        position: 0,
        name: 'Vývoj webu',
        quantity: '1',
        unit_name: 'ks',
        unit_price: 1000000,
        vat_rate_bps: 0,
        base: 1000000,
        vat: 0,
        total: 1000000,
      },
    ],
    vat_recap: [],
    payments: [],
    attachments: [],
    warnings: [],
    public_token: 'pub-token',
    created_at: TS,
    updated_at: TS,
    ...overrides,
  }
}
