// Jednotná chyba API. Backend (huma) vrací RFC 9457 problem+json:
// { title, status, detail, errors: [{ location: "body.email", message, value }] }

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

  /** Síťová chyba (backend nedostupný). */
  get isNetworkError(): boolean {
    return this.status === 0
  }
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
}

function problemMessage(status: number, problem: Problem): string {
  return problem.detail || statusMessages[status] || problem.title || `Chyba ${status}`
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
