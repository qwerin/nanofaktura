import { useEffect, useState } from 'react'

/** Aktuální čas obnovovaný po `intervalMs` (relativní časy typu „před 5 min“). */
export function useNow(intervalMs = 60_000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(t)
  }, [intervalMs])
  return now
}
