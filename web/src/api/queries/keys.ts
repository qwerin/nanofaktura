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
}
