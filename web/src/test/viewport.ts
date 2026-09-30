/** Emulace šířky okna pro `window.matchMedia` (useIsMobile, Sidebar). Výchozí = desktop. */
export const viewport = { width: 1440, dark: false }

export function setMobile(mobile = true) {
  viewport.width = mobile ? 390 : 1440
}

export function resetViewport() {
  viewport.width = 1440
  viewport.dark = false
}
