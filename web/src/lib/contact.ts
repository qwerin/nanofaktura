// Formátování kontaktních údajů (adresa, PSČ, odkazy tel:/mailto:/web).

export interface AddressParts {
  street?: string
  city?: string
  zip?: string
  country?: string
}

/** České/slovenské PSČ „11000“ → „110 00“; ostatní beze změny. */
export function formatZip(zip: string, country = 'CZ'): string {
  const z = zip.trim()
  if ((country === 'CZ' || country === 'SK') && /^\d{5}$/.test(z)) return `${z.slice(0, 3)} ${z.slice(3)}`
  return z
}

const countryNames = new Intl.DisplayNames(['cs'], { type: 'region' })

export function countryName(code: string): string {
  try {
    return countryNames.of(code.toUpperCase()) ?? code
  } catch {
    return code
  }
}

/** Řádky adresy: [„Ulice 1“, „110 00 Praha“, „Slovensko“ (jen mimo CZ)]. */
export function addressLines(a: AddressParts): string[] {
  const country = (a.country || 'CZ').toUpperCase()
  const cityLine = [formatZip(a.zip ?? '', country), a.city?.trim()].filter(Boolean).join(' ')
  const lines = [a.street?.trim() ?? '', cityLine]
  if (country !== 'CZ') lines.push(countryName(country))
  return lines.filter(Boolean)
}

/** Adresa na jeden řádek (pro kopírování a seznamy). */
export function formatAddress(a: AddressParts): string {
  return addressLines(a).join(', ')
}

/** Odkaz na web — doplní https:// když chybí schéma. */
export function webHref(web: string): string {
  const w = web.trim()
  return /^https?:\/\//i.test(w) ? w : `https://${w}`
}

/** Web bez schématu a koncového lomítka pro zobrazení. */
export function displayWeb(web: string): string {
  return web.trim().replace(/^https?:\/\//i, '').replace(/\/$/, '')
}

export function telHref(phone: string): string {
  return `tel:${phone.replace(/[^\d+]/g, '')}`
}
