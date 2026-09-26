import { useEffect } from 'react'

/**
 * Detekce otevřené softwarové klávesnice na mobilu (přes VisualViewport).
 * Nastaví `data-keyboard="open"` na <html> — CSS varianta `keyboard-open:` pak skryje tab bar
 * a sticky akční lišta se přilepí přímo nad klávesnici.
 */
export function KeyboardInsetsWatcher() {
  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return
    // Výška viewportu bez klávesnice; při změně šířky (rotace) se resetuje.
    let baseline = vv.height
    let width = vv.width
    const update = () => {
      if (vv.width !== width) {
        width = vv.width
        baseline = vv.height
      }
      baseline = Math.max(baseline, vv.height)
      const open = baseline - vv.height > 150
      document.documentElement.dataset.keyboard = open ? 'open' : 'closed'
      document.documentElement.style.setProperty(
        '--keyboard-inset',
        `${Math.max(0, window.innerHeight - vv.height - vv.offsetTop)}px`,
      )
    }
    update()
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  }, [])
  return null
}
