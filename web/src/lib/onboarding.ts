// Průvodce po registraci (SPEC §7.15): zobrazí se jednou, když má účet prázdný firemní profil.

import type { QueryClient } from '@tanstack/react-query'
import { api } from '@/api/client'
import { keys } from '@/api/queries/keys'

interface ProfileLike {
  registration_no: string
  street: string
  city: string
  email: string
}

/** Profil je prázdný = po registraci je vyplněný jen název. */
export function isProfileEmpty(a: ProfileLike): boolean {
  return !a.registration_no && !a.street && !a.city && !a.email
}

/** Průvodce už byl pro účet zobrazen (dokončen nebo přeskočen) — příznak `onboarded_at` na účtu. */
export function isOnboarded(a: { onboarded_at?: string | null }): boolean {
  return Boolean(a.onboarded_at)
}

/**
 * Označí průvodce za zobrazený (PATCH `onboarded: true`) a uloží nový účet do cache, aby guard
 * v `/a/$slug` hned viděl `onboarded_at`. Chyba se ignoruje — nejhůř se průvodce nabídne znovu.
 */
export async function markOnboarded(qc: QueryClient, slug: string): Promise<void> {
  try {
    const { data } = await api.PATCH('/api/accounts/{slug}', { params: { path: { slug } }, body: { onboarded: true } })
    if (data) qc.setQueryData(keys.accountDetail(slug), data)
  } catch {
    // ignore
  }
}
