import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { authQueries } from '@/api/queries/auth'

/**
 * Aktuální účet podle `$slug` v URL + přihlášený uživatel.
 * Použitelné jen uvnitř `/a/$slug/…` (auth guard zaručí, že `me` existuje a účet je v něm).
 */
export function useCurrentAccount() {
  const { slug } = useParams({ from: '/a/$slug' })
  const { data: me } = useQuery(authQueries.me())
  const accounts = me?.accounts ?? []
  const membership = accounts.find((a) => a.slug === slug)
  return {
    slug,
    me: me ?? null,
    user: me?.user ?? null,
    accounts,
    membership,
    isOwner: membership?.role === 'owner',
  }
}
