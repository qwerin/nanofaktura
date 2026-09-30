// Shared Playwright fixtures: every test gets its own freshly registered owner
// (cookie in the browser context, so `page` is logged in), a small typed API
// client for seeding data, the captured mail inbox and a CSP/page-error guard.
import { randomBytes } from 'node:crypto'
import { test as base, expect, type APIRequestContext, type Page } from '@playwright/test'
import { startFakes, type RunningFakes } from './fakes'
import { startServer, type Instance, type ServerOptions } from './server'

export { expect }

export const baseURL = () => {
  const u = process.env.E2E_BASE_URL
  if (!u) throw new Error('E2E_BASE_URL not set — run through playwright (global-setup)')
  return u
}
export const fakesURL = () => process.env.E2E_FAKES_URL!

export const PASSWORD = 'heslo-e2e-1234'

export function uniqueEmail(prefix = 'user'): string {
  return `${prefix}-${randomBytes(5).toString('hex')}@e2e.test`
}

/** Valid Czech IČO (mod-11 checksum) starting with the given digit. */
export function makeIco(first = '2'): string {
  for (;;) {
    const body = first + String(Math.floor(Math.random() * 1e6)).padStart(6, '0')
    let sum = 0
    for (let i = 0; i < 7; i++) sum += Number(body[i]) * (8 - i)
    const check = (11 - (sum % 11)) % 10
    // checksum 10 → 0 and 11 → 1 are ambiguous in the spec; avoid them
    if ((11 - (sum % 11)) === 10 || (11 - (sum % 11)) === 11) continue
    return body + String(check)
  }
}

export class ApiError extends Error {}

/** JSON API client on top of a Playwright request context (shares cookies with the page). */
export class Api {
  constructor(
    readonly request: APIRequestContext,
    readonly slug = '',
    readonly base = baseURL(),
  ) {}

  withSlug(slug: string) {
    return new Api(this.request, slug, this.base)
  }

  /** Path relative to the current account (`/subjects` → `/api/accounts/{slug}/subjects`). */
  acct(path: string) {
    return `/api/accounts/${this.slug}${path}`
  }

  async call<T = any>(method: string, path: string, body?: unknown, expected?: number): Promise<T> {
    const res = await this.request.fetch(this.base + path, {
      method,
      data: body === undefined ? undefined : body,
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    })
    const text = await res.text()
    if (expected ? res.status() !== expected : !res.ok()) {
      throw new ApiError(`${method} ${path} → ${res.status()}: ${text}`)
    }
    return (text && res.headers()['content-type']?.includes('json') ? JSON.parse(text) : text) as T
  }

  get<T = any>(path: string) {
    return this.call<T>('GET', path)
  }
  post<T = any>(path: string, body: unknown = {}, expected?: number) {
    return this.call<T>('POST', path, body, expected)
  }
  patch<T = any>(path: string, body: unknown) {
    return this.call<T>('PATCH', path, body)
  }

  // --- seeding helpers (account-scoped) ---

  subject(data: Record<string, unknown> = {}) {
    return this.post<Subject>(this.acct('/subjects'), {
      name: 'Odběratel s.r.o.',
      email: uniqueEmail('client'),
      street: 'Dlouhá 1',
      city: 'Brno',
      zip: '60200',
      country: 'CZ',
      ...data,
    })
  }

  invoice(subjectId: number, data: Record<string, unknown> = {}) {
    return this.post<Invoice>(this.acct('/invoices'), {
      subject_id: subjectId,
      lines: [{ name: 'Konzultace', quantity: '2', unit_price: 150000, vat_rate_bps: 2100 }],
      ...data,
    })
  }

  action(invoiceId: number, action: string) {
    return this.post<Invoice>(this.acct(`/invoices/${invoiceId}/actions/${action}`))
  }

  bankAccount(data: Record<string, unknown> = {}) {
    return this.post<{ id: number }>(this.acct('/bank-accounts'), {
      name: 'Hlavní účet',
      number: '2000145399/0800',
      is_default: true,
      ...data,
    })
  }
}

export interface Subject {
  id: number
  name: string
  email: string
}
export interface Invoice {
  id: number
  number: string
  variable_symbol: string
  status: string
  total: number
  public_token: string
  document_type: string
}

export interface Owner {
  email: string
  password: string
  name: string
  accountName: string
  slug: string
  api: Api
}

export interface RegisterOptions {
  email?: string
  name?: string
  accountName?: string
  onboarded?: boolean
  vatPayer?: boolean
  account?: Record<string, unknown>
}

/** Registers a user + account through the API; the session cookie lands in `request`'s cookie jar. */
export async function register(request: APIRequestContext, opts: RegisterOptions = {}, base = baseURL()): Promise<Owner> {
  const email = opts.email ?? uniqueEmail('owner')
  const name = opts.name ?? 'Jana Testová'
  const accountName = opts.accountName ?? `Firma ${randomBytes(3).toString('hex')}`
  const api = new Api(request, '', base)
  const me = await api.post<{ accounts: { slug: string }[] }>('/api/auth/register', {
    email,
    name,
    password: PASSWORD,
    account_name: accountName,
  })
  const slug = me.accounts[0].slug
  const acctApi = api.withSlug(slug)
  if (opts.onboarded !== false) {
    await acctApi.patch(`/api/accounts/${slug}`, {
      onboarded: true,
      registration_no: makeIco('2'),
      street: 'Hlavní 10',
      city: 'Praha',
      zip: '11000',
      country: 'CZ',
      email,
      ...(opts.vatPayer ? { vat_mode: 'vat_payer', vat_no: 'CZ12345679' } : {}),
      ...opts.account,
    })
  }
  return { email, password: PASSWORD, name, accountName, slug, api: acctApi }
}

export async function login(page: Page, email: string, password = PASSWORD) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Heslo', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Přihlásit se' }).click()
}

// --- captured e-mail ---

export interface CapturedMail {
  from: string
  to: string[]
  subject: string
  text: string
  html: string
  attachments: { filename: string; contentType: string; size: number }[]
}

export const mailbox = {
  async all(to: string): Promise<CapturedMail[]> {
    const res = await fetch(`${fakesURL()}/mail?to=${encodeURIComponent(to)}`)
    return (await res.json()) as CapturedMail[]
  },
  /** Waits for the n-th (1-based, default: first) e-mail to `to` matching `subject`. */
  async waitFor(to: string, subject?: RegExp, n = 1): Promise<CapturedMail> {
    let found: CapturedMail[] = []
    await expect
      .poll(
        async () => {
          found = (await mailbox.all(to)).filter((m) => !subject || subject.test(m.subject))
          return found.length
        },
        { message: `e-mail to ${to} ${subject ?? ''}`, timeout: 15_000 },
      )
      .toBeGreaterThanOrEqual(n)
    return found[n - 1]
  },
}

/** First URL in the mail text starting with the instance URL + path prefix (e.g. "/invite/"). */
export function linkFrom(mail: CapturedMail, pathPrefix: string, base = baseURL()): string {
  const re = new RegExp(`${base.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}${pathPrefix}[^\\s"'<>)]+`)
  const m = (mail.text + '\n' + mail.html).match(re)
  if (!m) throw new Error(`no ${pathPrefix} link in e-mail "${mail.subject}":\n${mail.text}`)
  return m[0]
}

// --- separate instances (pristine DB, rate limits on …) ---

export async function startInstance(opts: ServerOptions = {}): Promise<{ instance: Instance; fakes: RunningFakes | null; stop(): Promise<void> }> {
  // reuse the shared fakes (SMTP sink + registries) of the global setup
  const fakes = { url: fakesURL(), smtpPort: Number(process.env.E2E_SMTP_PORT) }
  const instance = await startServer(fakes, opts)
  return { instance, fakes: null, stop: () => instance.stop() }
}
export { startFakes }

// --- fixtures ---

type Fixtures = {
  owner: Owner
  api: Api
  isMobile: boolean
  pageErrors: string[]
}

export const test = base.extend<Fixtures>({
  baseURL: async ({}, use) => use(baseURL()),

  // Fails the test on CSP violations and uncaught errors in the page.
  pageErrors: [
    async ({ page }, use) => {
      const errors: string[] = []
      page.on('console', (msg) => {
        const t = msg.text()
        if (/Content Security Policy|Content-Security-Policy/i.test(t)) errors.push(`CSP: ${t}`)
      })
      page.on('pageerror', (err) => errors.push(`pageerror: ${err.message}`))
      await page.addInitScript(() => {
        document.addEventListener('securitypolicyviolation', (e) => {
          console.error(`Content Security Policy violation: ${e.violatedDirective} ${e.blockedURI}`)
        })
      })
      await use(errors)
      expect(errors, 'CSP violations / page errors').toEqual([])
    },
    { auto: true },
  ],

  owner: async ({ page }, use) => {
    await use(await register(page.request))
  },

  api: async ({ owner }, use) => use(owner.api),

  isMobile: async ({}, use, testInfo) => use(testInfo.project.name === 'mobile'),
})

/** Asserts the page has no horizontal scroll (mobile layouts). */
export async function expectNoHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow, 'horizontal overflow in px').toBeLessThanOrEqual(0)
}
