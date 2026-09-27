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

// --- Invoices ---
export type Invoice = ResponseBody<'/api/accounts/{slug}/invoices/{id}', 'get'>
export type InvoiceList = ResponseBody<'/api/accounts/{slug}/invoices', 'get'>
export type InvoiceSummary = ListItem<InvoiceList>
export type InvoiceLine = Invoice['lines'][number]
export type InvoicePayment = Invoice['payments'][number]
export type VatRecapItem = Invoice['vat_recap'][number]
export type CreateInvoiceInput = RequestBody<'/api/accounts/{slug}/invoices', 'post'>
export type UpdateInvoiceInput = RequestBody<'/api/accounts/{slug}/invoices/{id}', 'patch'>
export type InvoiceLineInput = NonNullable<CreateInvoiceInput['lines']>[number]
export type InvoiceStatus = Invoice['status']
export type DocumentType = Invoice['document_type']
export type PaymentMethod = Invoice['payment_method']
export type InvoiceAction = Operation<
  '/api/accounts/{slug}/invoices/{id}/actions/{action}',
  'post'
>['parameters']['path']['action']
export type InvoiceListQuery = NonNullable<
  Operation<'/api/accounts/{slug}/invoices', 'get'>['parameters']['query']
>
export type CreatePaymentInput = RequestBody<'/api/accounts/{slug}/invoices/{id}/payments', 'post'>
export type PaymentResult = ResponseBody<'/api/accounts/{slug}/invoices/{id}/payments', 'post'>

// --- Dashboard ---
export type Dashboard = ResponseBody<'/api/accounts/{slug}/dashboard', 'get'>
// --- Expenses (náklady) ---
export type Expense = ResponseBody<'/api/accounts/{slug}/expenses/{id}', 'get'>
export type ExpenseSummary = ListItem<ResponseBody<'/api/accounts/{slug}/expenses', 'get'>>
export type ExpenseStatus = Expense['status']
export type ExpenseLine = Expense['lines'][number]
export type ExpensePayment = Expense['payments'][number]
export type ExpenseVatRecapItem = Expense['vat_recap'][number]
export type CreateExpenseInput = RequestBody<'/api/accounts/{slug}/expenses', 'post'>
export type UpdateExpenseInput = RequestBody<'/api/accounts/{slug}/expenses/{id}', 'patch'>
export type ExpenseLineInput = NonNullable<CreateExpenseInput['lines']>[number]
export type CreateExpensePaymentInput = RequestBody<'/api/accounts/{slug}/expenses/{id}/payments', 'post'>
export type ExpensePaymentMethod = NonNullable<CreateExpenseInput['payment_method']>
export type ExpenseListFilters = Omit<
  NonNullable<paths['/api/accounts/{slug}/expenses']['get']['parameters']['query']>,
  'page' | 'per_page'
>

// --- Price items (ceník) a sklad ---
export type PriceItem = ResponseBody<'/api/accounts/{slug}/price-items/{id}', 'get'>
export type CreatePriceItemInput = RequestBody<'/api/accounts/{slug}/price-items', 'post'>
export type UpdatePriceItemInput = RequestBody<'/api/accounts/{slug}/price-items/{id}', 'patch'>
export type PriceItemListFilters = Omit<
  NonNullable<paths['/api/accounts/{slug}/price-items']['get']['parameters']['query']>,
  'page' | 'per_page'
>
export type StockMove = ListItem<ResponseBody<'/api/accounts/{slug}/price-items/{id}/stock-moves', 'get'>>
export type CreateStockMoveInput = RequestBody<'/api/accounts/{slug}/price-items/{id}/stock-moves', 'post'>

// --- Attachments (přílohy) ---
export type Attachment = ResponseBody<'/api/accounts/{slug}/attachments/{id}', 'get'>
export type AttachmentOwnerType = Attachment['owner_type']

// --- Tým: členové a pozvánky ---
export type Member = ListItem<ResponseBody<'/api/accounts/{slug}/members', 'get'>>
export type MemberRole = Member['role']
export type UpdateMemberInput = RequestBody<'/api/accounts/{slug}/members/{user_id}', 'patch'>
export type Invitation = ListItem<ResponseBody<'/api/accounts/{slug}/invitations', 'get'>>
export type CreateInvitationInput = RequestBody<'/api/accounts/{slug}/members/invite', 'post'>
export type InvitationInfo = ResponseBody<'/api/invitations/{token}', 'get'>

// --- Vzhled dokladů ---
export type PdfPreviewQuery = NonNullable<
  Operation<'/api/accounts/{slug}/pdf-preview', 'get'>['parameters']['query']
>
