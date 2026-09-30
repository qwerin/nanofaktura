import { useCallback, useSyncExternalStore } from 'react'
import changelogSource from '../../../CHANGELOG.md?raw'
import { parseChangelog, unseenEntries } from '@/lib/changelog'

/** Novinky zabalené do buildu (CHANGELOG.md v kořeni repa), od nejnovější. */
export const newsEntries = parseChangelog(changelogSource)

const KEY = 'nf.news.lastSeen'
const listeners = new Set<() => void>()

function readLastSeen(): string | null {
  try {
    return localStorage.getItem(KEY)
  } catch {
    return null
  }
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  const onStorage = (e: StorageEvent) => e.key === KEY && cb()
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(cb)
    window.removeEventListener('storage', onStorage)
  }
}

/**
 * Stav přečtení novinek (per prohlížeč). `lastSeen` se čte PŘED `markSeen`,
 * takže stránka Novinky může zvýraznit, co bylo nové při otevření.
 */
export function useNews() {
  const lastSeen = useSyncExternalStore(subscribe, readLastSeen, () => null)
  const unseen = unseenEntries(newsEntries, lastSeen)
  const markSeen = useCallback(() => {
    const latest = newsEntries[0]?.date
    if (!latest || readLastSeen() === latest) return
    try {
      localStorage.setItem(KEY, latest)
    } catch {
      // bez localStorage (soukromé okno) se tečka zobrazí znovu — nevadí
    }
    listeners.forEach((l) => l())
  }, [])
  return { entries: newsEntries, lastSeen, unseenCount: unseen.length, markSeen }
}
