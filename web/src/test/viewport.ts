// Emulace šířky okna v jsdom:
// - `window.matchMedia` (useIsMobile, Sidebar) čte `viewport.width` (setup.ts)
// - Tailwind třídy pro viditelnost (`hidden`, `md:hidden`, `max-md:hidden`, `md:flex` …) dostanou odpovídající
//   `display`, takže Testing Library (getByRole) vidí jen mobilní, nebo jen desktopovou variantu — jako prohlížeč.
export const viewport = { width: 1440, dark: false }

const BREAKPOINTS = { sm: 640, md: 768, lg: 1024, xl: 1280 } as const
const DISPLAYS = ['flex', 'block', 'inline-flex', 'grid', 'inline', 'inline-block', 'table-cell', 'contents'] as const

function css(width: number): string {
  const esc = (cls: string) => cls.replace(/:/g, '\\:')
  const rules = ['.hidden { display: none; }']
  for (const [bp, min] of Object.entries(BREAKPOINTS)) {
    if (width >= min) {
      rules.push(`.${esc(`${bp}:hidden`)} { display: none; }`)
      for (const d of DISPLAYS) rules.push(`.${esc(`${bp}:${d}`)} { display: ${d}; }`)
    } else {
      rules.push(`.${esc(`max-${bp}:hidden`)} { display: none; }`)
    }
  }
  return rules.join('\n')
}

export function applyViewport() {
  let style = document.getElementById('test-viewport') as HTMLStyleElement | null
  if (!style) {
    style = document.createElement('style')
    style.id = 'test-viewport'
    document.head.appendChild(style)
  }
  style.textContent = css(viewport.width)
}

/** Mobil (390 px) nebo desktop (1440 px). Volat před renderem. */
export function setMobile(mobile = true) {
  viewport.width = mobile ? 390 : 1440
  applyViewport()
}

export function resetViewport() {
  viewport.width = 1440
  viewport.dark = false
  applyViewport()
}
