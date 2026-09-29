// Konvence query klíčů (TanStack Query):
//
//   ['auth', ...]                 — přihlášený uživatel, tokeny (nezávislé na účtu)
//   ['accounts']                  — seznam účtů uživatele
//   ['a', slug, 'detail']         — detail účtu (firemní profil)
//   ['a', slug, '<resource>', ...] — všechny doménové zdroje účtu (invoices, subjects, …)
//
// Díky prefixu ['a', slug] jde jedním voláním zneplatnit vše pro daný účet:
//   queryClient.invalidateQueries({ queryKey: keys.account(slug) })
//
// Nové zdroje přidávejte sem jako továrny vracející `as const` n-tice, např.:
//   invoices: (slug: string) => [...keys.account(slug), 'invoices'] as const,
//   invoiceList: (slug: string, search: InvoiceSearch) => [...keys.invoices(slug), 'list', search] as const,

export const keys = {
  auth: ['auth'] as const,
  authStatus: () => [...keys.auth, 'status'] as const,
  me: () => [...keys.auth, 'me'] as const,
  tokens: () => [...keys.auth, 'tokens'] as const,

  accounts: () => ['accounts'] as const,
  /** Prefix všech dat účtu. */
  account: (slug: string) => ['a', slug] as const,
  accountDetail: (slug: string) => [...keys.account(slug), 'detail'] as const,

  subjects: (slug: string) => [...keys.account(slug), 'subjects'] as const,
  subjectList: (slug: string, filters: { query?: string; type?: string }) =>
    [...keys.subjects(slug), 'list', filters] as const,
  /** Jednorázové hledání (kontrola duplicit, našeptávač) — nestránkované. */
  subjectSearch: (slug: string, query: string, perPage: number) =>
    [...keys.subjects(slug), 'list', 'search', query, perPage] as const,
  subjectDetail: (slug: string, id: number) => [...keys.subjects(slug), 'detail', id] as const,
  /** Faktury kontaktu — pod prefixem `invoices`, aby je zneplatnily i mutace faktur. */
  subjectInvoices: (slug: string, subjectId: number) =>
    [...keys.account(slug), 'invoices', 'by-subject', subjectId] as const,

  bankAccounts: (slug: string) => [...keys.account(slug), 'bank-accounts'] as const,

  numberFormats: (slug: string) => [...keys.account(slug), 'number-formats'] as const,
  numberFormatPreview: (slug: string, id: number, format: string) =>
    [...keys.numberFormats(slug), 'preview', id, format] as const,

  /** ARES nezávisí na účtu. */
  ares: (ico: string) => ['ares', ico] as const,

  // Faktury + přehled
  invoices: (slug: string) => [...keys.account(slug), 'invoices'] as const,
  invoiceList: (slug: string, filters: object) => [...keys.invoices(slug), 'list', filters] as const,
  invoiceDetail: (slug: string, id: number) => [...keys.invoices(slug), 'detail', id] as const,
  dashboard: (slug: string, year?: number) => [...keys.account(slug), 'dashboard', year ?? 'current'] as const,
  // --- Náklady ---
  expenses: (slug: string) => [...keys.account(slug), 'expenses'] as const,
  expenseList: (slug: string, filters: object) => [...keys.expenses(slug), 'list', filters] as const,
  expenseDetail: (slug: string, id: number) => [...keys.expenses(slug), 'detail', id] as const,
  expenseCategories: (slug: string, query: string) => [...keys.expenses(slug), 'categories', query] as const,
  /** Našeptávač dodavatelů ve formuláři nákladu (pod prefixem `subjects` → zneplatní se se změnou kontaktů). */
  expenseSupplierSearch: (slug: string, query: string) =>
    [...keys.account(slug), 'subjects', 'expense-supplier-search', query] as const,
  expenseSupplier: (slug: string, id: number) => [...keys.account(slug), 'subjects', 'expense-supplier', id] as const,

  // --- Ceník a sklad ---
  priceItems: (slug: string) => [...keys.account(slug), 'price-items'] as const,
  priceItemList: (slug: string, filters: object) => [...keys.priceItems(slug), 'list', filters] as const,
  priceItemDetail: (slug: string, id: number) => [...keys.priceItems(slug), 'detail', id] as const,
  stockMoves: (slug: string, id: number) => [...keys.priceItems(slug), 'stock-moves', id] as const,

  // --- Přílohy ---
  attachments: (slug: string) => [...keys.account(slug), 'attachments'] as const,
  attachmentList: (slug: string, ownerType: string, ownerId: number) =>
    [...keys.attachments(slug), 'list', ownerType, ownerId] as const,

  // Tým: členové a pozvánky
  members: (slug: string) => [...keys.account(slug), 'members'] as const,
  invitations: (slug: string) => [...keys.account(slug), 'invitations'] as const,
  /** Veřejný detail pozvánky podle tokenu z e-mailu (nezávislé na účtu). */
  invitationInfo: (token: string) => ['invitation', token] as const,

  // Vzhled dokladů (náhled PDF)
  pdfPreview: (slug: string, params: object) => [...keys.account(slug), 'pdf-preview', params] as const,
  // Šablony, pravidelné faktury, e-maily
  templates: (slug: string) => [...keys.account(slug), 'templates'] as const,
  templateList: (slug: string, filters: object) => [...keys.templates(slug), 'list', filters] as const,
  templateDetail: (slug: string, id: number) => [...keys.templates(slug), 'detail', id] as const,
  recurring: (slug: string) => [...keys.account(slug), 'recurring'] as const,
  recurringList: (slug: string, filters: object) => [...keys.recurring(slug), 'list', filters] as const,
  recurringDetail: (slug: string, id: number) => [...keys.recurring(slug), 'detail', id] as const,
  /** Historie e-mailů faktury — pod prefixem `invoices`, aby ji obnovily i mutace faktur. */
  invoiceEmails: (slug: string, invoiceId: number) => [...keys.invoices(slug), 'emails', invoiceId] as const,
  emailPreview: (slug: string, params: object) => [...keys.account(slug), 'email-preview', params] as const,

  // --- Banka ---
  bankTransactions: (slug: string) => [...keys.account(slug), 'bank-transactions'] as const,
  bankTransactionList: (slug: string, filters: object) => [...keys.bankTransactions(slug), 'list', filters] as const,
  /** Počty záznamů (per_page=1 → total) — pod prefixem transakcí, zneplatní se s nimi. */
  bankTransactionCount: (slug: string, filters: object) => [...keys.bankTransactions(slug), 'count', filters] as const,
  /** Registr plátců DPH pro kontakt (pod prefixem `subjects`). */
  subjectVatStatus: (slug: string, subjectId: number) => [...keys.subjects(slug), 'vat-status', subjectId] as const,
}
