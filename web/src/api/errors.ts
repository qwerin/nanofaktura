// Jednotná chyba API. Backend (huma) vrací RFC 9457 problem+json:
// { title, status, detail, code, errors: [{ location: "body.email", message, value }] }
// `code` je strojově čitelný důvod (internal/api/errors.go) — hlášky pro uživatele se
// odvozují z něj (codeMessages), nikdy z anglického `detail`.

import type { FieldValues, Path, UseFormSetError } from 'react-hook-form'

export interface ProblemDetail {
  location?: string
  message?: string
  value?: unknown
}

export interface Problem {
  type?: string
  title?: string
  status?: number
  detail?: string
  /** Strojově čitelný důvod, např. `has_invoices`, `locked` (viz internal/api/errors.go). */
  code?: string
  instance?: string
  errors?: ProblemDetail[] | null
}

export class ApiError extends Error {
  readonly status: number
  readonly problem: Problem

  constructor(status: number, problem: Problem) {
    super(problemMessage(status, problem))
    this.name = 'ApiError'
    this.status = status
    this.problem = problem
  }

  /** Strojově čitelný důvod chyby (`''` když chybí). */
  get code(): string {
    return this.problem.code ?? ''
  }

  /** Síťová chyba (backend nedostupný). */
  get isNetworkError(): boolean {
    return this.status === 0
  }
}

/** Česká hláška ke kódu chyby (doménové 409/403/422/410/504). */
export const codeMessages: Record<string, string> = {
  already_exists: 'Záznam se stejným označením už existuje.',
  email_taken: 'Tento e-mail už je zaregistrovaný.',
  signup_disabled: 'Registrace nových účtů je vypnutá.',
  locked: 'Doklad je zamčený. Nejdřív ho odemkněte.',
  has_payments: 'Doklad má platby. Nejdřív je smažte.',
  has_invoices: 'Kontakt má vystavené doklady, nelze ho smazat.',
  not_editable: 'Stornovaný nebo nedobytný doklad nelze upravit.',
  invalid_transition: 'Tuto akci nelze v aktuálním stavu dokladu provést.',
  no_number_format: 'Chybí číselná řada pro tento typ dokladu. Založte ji v nastavení.',
  referenced: 'Na doklad odkazuje jiný doklad (dobropis nebo vyúčtování). Nejdřív smažte ten.',
  used_by_template: 'Kontakt je použitý v šabloně faktury. Nejdřív šablonu smažte nebo změňte.',
  used_by_recurring: 'Šablonu používá pravidelná faktura. Nejdřív smažte pravidelnou fakturu.',
  not_payable: 'K dokladu v tomto stavu nelze přidat platbu.',
  nothing_to_pay: 'Doklad je už celý uhrazený.',
  final_exists: 'Vyúčtování této zálohy už existuje.',
  correction_invoice_only: 'Dobropis lze vystavit jen k faktuře.',
  correction_template: 'Dobropis nelze uložit jako šablonu.',
  sync_not_configured: 'Automatické stahování není pro tento účet nastavené.',
  zero_amount: 'Nulovou transakci nelze spárovat.',
  already_matched: 'Transakce je už spárovaná. Nejdřív párování zrušte.',
  not_matched: 'Transakce není spárovaná.',
  automatic_todo: 'Automatický úkol lze jen splnit nebo znovu otevřít.',
  is_default: 'Výchozí řadu nelze smazat ani zrušit. Nejdřív nastavte jako výchozí jinou řadu.',
  last_of_type: 'Poslední řadu typu dokladu nelze smazat.',
  stock_not_tracked: 'U položky se nesleduje sklad. Nejdřív sledování zapněte.',
  generated_move: 'Pohyb vznikl z dokladu — upravte doklad.',
  not_vat_payer: 'DPH přehledy jsou jen pro plátce DPH.',
  missing_tax_office: 'Chybí kód finančního úřadu nebo české DIČ v nastavení firmy.',
  last_owner: 'Účet musí mít alespoň jednoho vlastníka.',
  owner_only: 'Vlastníky může spravovat jen jiný vlastník.',
  already_member: 'Tento uživatel už je členem účtu.',
  already_joined: 'Už jste členem tohoto účtu.',
  invitation_expired: 'Pozvánka vypršela nebo už byla použita.',
  invitation_email_mismatch: 'Pozvánka byla poslána na jinou e-mailovou adresu.',
  recurring_ended: 'Pravidelná faktura už skončila. Nejdřív změňte datum konce.',
  template_missing: 'Šablona pravidelné faktury už neexistuje.',
  wrong_password: 'Současné heslo není správné.',
  no_vat_no: 'Kontakt nemá české DIČ.',
  fio_token_rejected: 'Fio odmítlo API token (neplatný, prošlý nebo bez oprávnění). Zadejte nový.',
  fio_too_many_transactions: 'V období je příliš mnoho transakcí. Nastavte pozdější začátek stahování.',
  upstream_timeout: 'Externí služba neodpověděla včas, zkuste to znovu.',
}

const statusMessages: Record<number, string> = {
  0: 'Server je nedostupný. Zkontrolujte připojení.',
  400: 'Neplatný požadavek.',
  401: 'Přihlášení vypršelo. Přihlaste se prosím znovu.',
  403: 'K této akci nemáte oprávnění.',
  404: 'Záznam nebyl nalezen.',
  409: 'Akci nelze v aktuálním stavu provést.',
  422: 'Zkontrolujte zadané údaje.',
  429: 'Příliš mnoho požadavků, zkuste to za chvíli.',
  500: 'Na serveru nastala chyba.',
  502: 'Externí služba je nedostupná.',
  503: 'Služba je dočasně nedostupná.',
  504: 'Externí služba neodpověděla včas, zkuste to znovu.',
}

function problemMessage(status: number, problem: Problem): string {
  const byCode = problem.code ? codeMessages[problem.code] : undefined
  return byCode || problem.detail || statusMessages[status] || problem.title || `Chyba ${status}`
}

/** Je `err` chyba API s některým z kódů? */
export function hasErrorCode(err: unknown, ...codes: string[]): boolean {
  return isApiError(err) && codes.includes(err.code)
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError
}

/** Text chyby pro toast / hlášku ve formuláři. */
export function errorMessage(err: unknown): string {
  if (isApiError(err)) return err.message
  if (err instanceof Error && err.message) return err.message
  return 'Nastala neočekávaná chyba.'
}

/**
 * Promítne validační chyby 422 do polí formuláře (`body.email` → `email`,
 * `body.lines[0].name` → `lines.0.name`). Vrací `true`, pokud se aspoň jedna chyba namapovala.
 */
export function applyProblemToForm<T extends FieldValues>(
  err: unknown,
  setError: UseFormSetError<T>,
  fields?: readonly string[],
): boolean {
  if (!isApiError(err) || !err.problem.errors?.length) return false
  let mapped = false
  for (const e of err.problem.errors) {
    if (!e.location?.startsWith('body.')) continue
    const path = e.location
      .slice('body.'.length)
      .replace(/\[(\d+)\]/g, '.$1')
    if (fields && !fields.includes(path.split('.')[0] ?? '')) continue
    setError(path as Path<T>, { type: 'server', message: e.message ?? 'Neplatná hodnota' })
    mapped = true
  }
  return mapped
}
