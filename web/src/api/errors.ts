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
  proforma_settled:
    'Záloha je už vyúčtovaná konečnou fakturou. Platby a změny zapisujte na tu fakturu (nebo ji nejdřív smažte).',
  advance_payment:
    'Tato platba je převzatá ze zálohové faktury. Smažte ji na zálohové faktuře, odsud zmizí sama.',
  tax_document_fixed:
    'Daňový doklad k přijaté platbě odpovídá platbě zálohy — částky ani měnu nelze měnit. Při chybě smažte platbu na zálohové faktuře.',
  currency_has_payments: 'Doklad už má platby, měnu nelze změnit. Nejdřív platby smažte.',
  correction_required:
    'Odeslaný daňový doklad nelze stornovat. Vystavte k němu opravný daňový doklad (dobropis).',
  monthly_control_statement:
    'Právnická osoba podává kontrolní hlášení vždy za měsíc, i při čtvrtletním přiznání. Vyberte měsíc.',
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
  invitation_revoked: 'Pozvánka už neplatí — kdo ji poslal, už nemá oprávnění tuto roli udělit. Požádejte o novou.',
  recurring_ended: 'Pravidelná faktura už skončila. Nejdřív změňte datum konce.',
  template_missing: 'Šablona pravidelné faktury už neexistuje.',
  wrong_password: 'Současné heslo není správné.',
  no_vat_no: 'Kontakt nemá české DIČ.',
  fio_token_rejected: 'Fio odmítlo API token (neplatný, prošlý nebo bez oprávnění). Zadejte nový.',
  fio_too_many_transactions: 'V období je příliš mnoho transakcí. Nastavte pozdější začátek stahování.',
  upstream_timeout: 'Externí služba neodpověděla včas, zkuste to znovu.',
  unsupported_backup_version: 'Záloha pochází z novější verze NanoFaktury. Nejdřív aktualizujte tuto instanci.',
  corrupt_backup: 'Soubor není platná záloha NanoFaktury nebo je poškozený.',
  backup_too_large: 'Záloha je větší, než tato instance dovoluje importovat (NANOFAKTURA_IMPORT_MAX_MB).',
  too_large: 'Soubor je příliš velký.',
  reset_expired: 'Odkaz pro obnovu hesla už byl použit nebo vypršel. Požádejte o nový.',
  invalid_code: 'Kód není správný. Zkontrolujte ho a zkuste to znovu.',
  two_factor_expired: 'Přihlášení vypršelo nebo bylo příliš mnoho pokusů. Zadejte znovu e-mail a heslo.',
  webauthn_failed: 'Bezpečnostní klíč se nepodařilo ověřit.',
  totp_enabled: 'Ověřovací aplikace už je zapnutá.',
  totp_not_pending: 'Nastavení ověřovací aplikace vypršelo. Začněte znovu.',
  two_factor_disabled: 'Dvoufázové ověření není zapnuté.',
  not_instance_admin: 'Tato část je jen pro správce instance (e-mail uvedený v NANOFAKTURA_ADMIN_EMAILS a ověřený).',
  verification_expired: 'Ověřovací odkaz už byl použit nebo vypršel. Požádejte o nový.',
  email_already_verified: 'E-mailová adresa už je ověřená.',
  rate_limited: 'Příliš mnoho pokusů v krátké době. Chvíli počkejte a zkuste to znovu.',
  export_in_progress: 'Jiný export tohoto účtu právě běží. Počkejte, až doběhne, a zkuste to znovu.',
  invoice_draft: 'Doklad je zatím koncept. Nejdřív ho vystavte.',
  oauth_invalid_client: 'Aplikace není u NanoFaktury registrovaná. Zkuste ji připojit znovu.',
  oauth_invalid_redirect: 'Aplikace žádá o návrat na adresu, kterou při registraci neuvedla. Přístup nelze povolit.',
  oauth_session_required: 'Přístup aplikaci povolte přihlášení v NanoFaktuře, ne API tokenem.',
  setup_token_invalid: 'Pro první registraci je potřeba správný instalační token.',
  password_too_long: 'Heslo je příliš dlouhé (nejvýše 72 bajtů — znaky s diakritikou se počítají dvakrát).',
  cross_origin_request: 'Požadavek byl odmítnut z bezpečnostních důvodů. Obnovte stránku a zkuste to znovu.',
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
