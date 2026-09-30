/** Veřejné stránky přihlášení — nikdy cíl přesměrování po přihlášení ani na /login po vypršení session. */
export const PUBLIC_AUTH_PAGES = ['/login', '/register', '/forgot-password', '/reset-password']

/** Povolí jen relativní cesty v rámci aplikace (ochrana proti open-redirect). */
export function safeRedirect(target: string | undefined, fallback = '/'): string {
  if (!target || !target.startsWith('/') || target.startsWith('//') || target.startsWith('/\\')) {
    return fallback
  }
  if (PUBLIC_AUTH_PAGES.some((p) => target.startsWith(p))) return fallback
  return target
}
