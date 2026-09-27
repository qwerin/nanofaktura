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
}
