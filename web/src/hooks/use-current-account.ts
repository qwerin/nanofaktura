import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { accountQueries } from '@/api/queries/accounts'
import { authQueries } from '@/api/queries/auth'
import { canEditRole, canManageSettingsRole } from '@/lib/roles'

/**
 * Aktuální účet podle `$slug` v URL + přihlášený uživatel a oprávnění jeho role.
 * Použitelné jen uvnitř `/a/$slug/…` (auth guard zaručí, že `me` existuje a účet je v něm).
 *
 * Oprávnění bere z `capabilities` detailu účtu (odvozuje je backend z role); dokud detail
 * není v cache, odvodí je z role v `me`:
 * - `canManageSettings` — nastavení firmy, bankovní účty, číselné řady, uživatelé (owner, admin)
 * - `canEdit` — doklady, kontakty, náklady (owner, admin, member); účetní (accountant) jen čte
 * - `canViewReports` — daňové přehledy (owner, admin, accountant)
 * - `canManageOwners` — správa vlastníků (owner)
 */
export function useCurrentAccount() {
  const { slug } = useParams({ from: '/a/$slug' })
  const { data: me } = useQuery(authQueries.me())
  const { data: account } = useQuery(accountQueries.detail(slug))
  const accounts = me?.accounts ?? []
  const membership = accounts.find((a) => a.slug === slug)
  const role = membership?.role
  const caps = account?.capabilities
  return {
    slug,
    me: me ?? null,
    user: me?.user ?? null,
    accounts,
    membership,
    role,
    canManageSettings: caps?.manage_settings ?? canManageSettingsRole(role),
    canEdit: caps?.edit ?? canEditRole(role),
    canViewReports: caps?.view_reports ?? (role === 'owner' || role === 'admin' || role === 'accountant'),
    canManageOwners: caps?.manage_owners ?? role === 'owner',
  }
}
