import { useSyncExternalStore } from 'react'

/** Chromium: událost, kterou lze později vyvolat jako systémový dialog „Instalovat aplikaci“. */
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

let deferred: BeforeInstallPromptEvent | null = null
let installed = false
const listeners = new Set<() => void>()
const emit = () => listeners.forEach((l) => l())

function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  return (
    window.matchMedia?.('(display-mode: standalone)').matches ||
    window.matchMedia?.('(display-mode: fullscreen)').matches ||
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  )
}

// Poslouchat hned při načtení modulu (importuje ho main.tsx) — událost přichází brzy.
if (typeof window !== 'undefined') {
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault() // vlastní tlačítko místo automatické lišty prohlížeče
    deferred = e as BeforeInstallPromptEvent
    emit()
  })
  window.addEventListener('appinstalled', () => {
    deferred = null
    installed = true
    emit()
  })
}

export type InstallMode =
  /** Běží jako nainstalovaná aplikace (nebo právě nainstalována) — nic nenabízet. */
  | 'installed'
  /** Prohlížeč umí systémový dialog instalace (Chrome, Edge, Samsung Internet). */
  | 'prompt'
  /** iPhone/iPad: jen ručně přes Sdílet → Přidat na plochu. */
  | 'ios'
  /** Jiný mobilní prohlížeč: ručně z menu prohlížeče. */
  | 'manual'
  /** Desktop bez podpory — nenabízet. */
  | 'none'

export function isIOS(ua = navigator.userAgent): boolean {
  // iPadOS se hlásí jako Mac — rozliší ho dotykové ovládání
  return /iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1)
}

function snapshot(): InstallMode {
  if (installed || isStandalone()) return 'installed'
  if (deferred) return 'prompt'
  if (isIOS()) return 'ios'
  if (window.matchMedia?.('(pointer: coarse)').matches) return 'manual'
  return 'none'
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

/**
 * Instalace jako aplikace (PWA): v okně bez adresního řádku, s ikonou na ploše.
 * `install()` u režimu `prompt` otevře systémový dialog a vrátí, zda uživatel potvrdil.
 */
export function useInstallApp(): {
  mode: InstallMode
  install: () => Promise<boolean>
} {
  const mode = useSyncExternalStore(subscribe, snapshot, () => 'none' as InstallMode)
  const install = async () => {
    const e = deferred
    if (!e) return false
    await e.prompt()
    const { outcome } = await e.userChoice
    deferred = null // jednorázová událost
    emit()
    return outcome === 'accepted'
  }
  return { mode, install }
}
