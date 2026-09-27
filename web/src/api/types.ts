// Doménové typy odvozené z vygenerovaného `schema.gen.ts` přes `paths`.
// Aplikace importuje typy odsud, ne z `components["schemas"]` — názvy schémat
// generuje backend (huma) a mohou se měnit, cesty a metody jsou kontrakt (SPEC).

import type { paths } from './schema.gen'

type HttpMethod = 'get' | 'post' | 'put' | 'patch' | 'delete'

type Operation<P extends keyof paths, M extends HttpMethod> = NonNullable<paths[P][M]>

type JsonContent<R> = R extends { content: { 'application/json': infer T } } ? T : never

/** JSON tělo úspěšné odpovědi (200/201). */
export type ResponseBody<P extends keyof paths, M extends HttpMethod> =
  Operation<P, M> extends { responses: infer R }
    ? R extends { 200: infer OK }
      ? JsonContent<OK>
      : R extends { 201: infer Created }
        ? JsonContent<Created>
        : never
    : never

/** JSON tělo požadavku. */
export type RequestBody<P extends keyof paths, M extends HttpMethod> =
  Operation<P, M> extends { requestBody?: infer B } ? JsonContent<NonNullable<B>> : never

type ListItem<L> = L extends { items: (infer T)[] | null } ? T : never

// --- Auth ---
export type AuthStatus = ResponseBody<'/api/auth/status', 'get'>
export type Me = ResponseBody<'/api/auth/me', 'get'>
export type MeUser = Me['user']
export type AccountMembership = NonNullable<Me['accounts']>[number]
export type LoginInput = RequestBody<'/api/auth/login', 'post'>
export type RegisterInput = RequestBody<'/api/auth/register', 'post'>
export type UpdateMeInput = RequestBody<'/api/auth/me', 'patch'>
export type ApiToken = ListItem<ResponseBody<'/api/auth/tokens', 'get'>>
export type ApiTokenCreated = ResponseBody<'/api/auth/tokens', 'post'>
export type CreateTokenInput = RequestBody<'/api/auth/tokens', 'post'>

// --- Accounts ---
export type Account = ResponseBody<'/api/accounts/{slug}', 'get'>
export type UpdateAccountInput = RequestBody<'/api/accounts/{slug}', 'patch'>
export type CreateAccountInput = RequestBody<'/api/accounts', 'post'>
export type VatMode = Account['vat_mode']

// --- Subjects (kontakty) ---
export type SubjectListResponse = ResponseBody<'/api/accounts/{slug}/subjects', 'get'>
export type Subject = ListItem<SubjectListResponse>
export type SubjectType = Subject['type']
export type CreateSubjectInput = RequestBody<'/api/accounts/{slug}/subjects', 'post'>
export type UpdateSubjectInput = RequestBody<'/api/accounts/{slug}/subjects/{id}', 'patch'>
export type AresSubject = ResponseBody<'/api/ares/{ico}', 'get'>

// --- Bank accounts ---
export type BankAccount = ListItem<ResponseBody<'/api/accounts/{slug}/bank-accounts', 'get'>>
export type CreateBankAccountInput = RequestBody<'/api/accounts/{slug}/bank-accounts', 'post'>
export type UpdateBankAccountInput = RequestBody<'/api/accounts/{slug}/bank-accounts/{id}', 'patch'>

// --- Number formats (číselné řady) ---
export type NumberFormat = ListItem<ResponseBody<'/api/accounts/{slug}/number-formats', 'get'>>
export type NumberFormatDocumentType = NumberFormat['document_type']
export type CreateNumberFormatInput = RequestBody<'/api/accounts/{slug}/number-formats', 'post'>
export type UpdateNumberFormatInput = RequestBody<'/api/accounts/{slug}/number-formats/{id}', 'patch'>
