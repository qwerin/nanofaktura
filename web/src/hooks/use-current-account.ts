import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { authQueries } from '@/api/queries/auth'
import { canEditRole, canManageSettingsRole } from '@/lib/roles'

/**
 * Aktuální účet podle `$slug` v URL + přihlášený uživatel a oprávnění jeho role.
 * Použitelné jen uvnitř `/a/$slug/…` (auth guard zaručí, že `me` existuje a účet je v něm).
 *
 * - `canManageSettings` — nastavení firmy, bankovní účty, číselné řady, uživatelé (owner, admin)
 * - `canEdit` — doklady, kontakty, náklady (owner, admin, member); účetní (accountant) jen čte
 */
export function useCurrentAccount() {
  const { slug } = useParams({ from: '/a/$slug' })
  const { data: me } = useQuery(authQueries.me())
  const accounts = me?.accounts ?? []
  const membership = accounts.find((a) => a.slug === slug)
  const role = membership?.role
  return {
    slug,
    me: me ?? null,
    user: me?.user ?? null,
    accounts,
    membership,
    role,
    canManageSettings: canManageSettingsRole(role),
    canEdit: canEditRole(role),
  }
}
