// Průvodce po registraci (SPEC §7.15): zobrazí se jednou, když má účet prázdný firemní profil.

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

const key = (slug: string) => `nf-onboarding:${slug}`

/** Průvodce už byl pro účet zobrazen (dokončen nebo přeskočen). */
export function isOnboardingSeen(slug: string): boolean {
  try {
    return localStorage.getItem(key(slug)) !== null
  } catch {
    // Bez localStorage (privátní režim…) průvodce nevnucujeme opakovaně.
    return true
  }
}

export function markOnboardingSeen(slug: string): void {
  try {
    localStorage.setItem(key(slug), new Date().toISOString())
  } catch {
    // ignore
  }
}
