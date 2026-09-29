import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { RateLimitedError, useSyncBankAccount } from '@/api/queries/bank'
import type { BankAccount } from '@/api/types'
import { classifySyncError, parseRetryAfter, syncErrorText } from './format'

/**
 * Ruční synchronizace Fio účtu. Hlídá limit Fio (1 dotaz / 30 s): po 429 drží odpočet
 * pro daný účet, tlačítko ukazuje „zkuste za N s“. 409/422 nabídnou přechod do nastavení.
 */
export function useBankSync(slug: string) {
  const navigate = useNavigate()
  const sync = useSyncBankAccount(slug)
  const [deadlines, setDeadlines] = useState<Record<number, number>>({})
  const [now, setNow] = useState(() => Date.now())
  const [pendingId, setPendingId] = useState<number | null>(null)

  const active = Object.values(deadlines).some((d) => d > now)
  useEffect(() => {
    if (!active) return
    const t = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(t)
  }, [active])

  const secondsLeft = (id: number) => Math.max(0, Math.ceil(((deadlines[id] ?? 0) - now) / 1000))

  const openSettings = () => void navigate({ to: '/a/$slug/settings/bank-accounts', params: { slug } })

  const run = (account: BankAccount) => {
    setPendingId(account.id)
    sync.mutate(account.id, {
      onSuccess: (r) => {
        // Po úspěchu platí limit Fio také — další dotaz nejdřív za 30 s.
        setDeadlines((d) => ({ ...d, [account.id]: Date.now() + 30_000 }))
        setNow(Date.now())
        const parts = [`${r.imported} nových`]
        if (r.matched) parts.push(`${r.matched} spárováno`)
        if (r.suggestions) parts.push(`${r.suggestions} s návrhem`)
        toast.success(r.imported ? `Staženo: ${parts.join(', ')}` : 'Žádné nové pohyby', { description: account.name })
      },
      onError: (err) => {
        const retry = err instanceof RateLimitedError ? parseRetryAfter(err.retryAfter) : undefined
        const e = classifySyncError(err, retry)
        if (e.kind === 'rate_limited') {
          setDeadlines((d) => ({ ...d, [account.id]: Date.now() + e.retryAfter * 1000 }))
          setNow(Date.now())
          toast.warning(syncErrorText(e))
          return
        }
        const settingsAction = { label: 'Nastavení', onClick: openSettings }
        toast.error(syncErrorText(e), e.kind === 'other' ? undefined : { action: settingsAction })
      },
      onSettled: () => setPendingId(null),
    })
  }

  return { run, pendingId, secondsLeft }
}
