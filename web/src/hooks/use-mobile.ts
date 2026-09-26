import { useSyncExternalStore } from "react"

/** Tailwind `md` breakpoint — pod ním je „mobilní“ layout (tab bar, drawery). */
export const MOBILE_BREAKPOINT = 768

const query = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

function subscribe(onChange: () => void) {
  const mql = window.matchMedia(query)
  mql.addEventListener("change", onChange)
  return () => mql.removeEventListener("change", onChange)
}

/** `true` pod `md` breakpointem. Synchronní (bez probliknutí desktopové varianty). */
export function useIsMobile(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(query).matches,
    () => false,
  )
}
