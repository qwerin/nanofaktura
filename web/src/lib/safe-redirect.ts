/** Povolí jen relativní cesty v rámci aplikace (ochrana proti open-redirect). */
export function safeRedirect(target: string | undefined, fallback = '/'): string {
  if (!target || !target.startsWith('/') || target.startsWith('//') || target.startsWith('/\\')) {
    return fallback
  }
  if (target.startsWith('/login') || target.startsWith('/register')) return fallback
  return target
}
