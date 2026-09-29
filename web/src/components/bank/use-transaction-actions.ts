import { useState } from 'react'
import { toast } from 'sonner'
import { useMatchTransaction, useSetIgnored, useUnmatchTransaction } from '@/api/queries/bank'
import type { BankTransaction, MatchSuggestion, MatchTransactionInput } from '@/api/types'
import { matchErrorText } from './format'

export interface MatchTarget {
  body: MatchTransactionInput
  /** „faktura 2026001“ — pro toast. */
  label: string
}

/** Akce nad transakcemi (párování, ignorování) s toasty a „Vrátit“. `busy[id]` = probíhající akce. */
export function useTransactionActions(slug: string) {
  const matchM = useMatchTransaction(slug)
  const unmatchM = useUnmatchTransaction(slug)
  const ignoreM = useSetIgnored(slug)
  const [busy, setBusy] = useState<Record<number, string>>({})

  const track = async (id: number, what: string, fn: () => Promise<unknown>) => {
    setBusy((b) => ({ ...b, [id]: what }))
    try {
      await fn()
    } finally {
      setBusy((b) => {
        const next = { ...b }
        delete next[id]
        return next
      })
    }
  }

  const unmatch = (t: BankTransaction, opts: { silent?: boolean } = {}) =>
    track(t.id, 'unmatch', async () => {
      try {
        await unmatchM.mutateAsync(t.id)
        if (!opts.silent) toast.success('Spárování zrušeno — úhrada u dokladu byla smazána')
      } catch (err) {
        toast.error(matchErrorText(err))
      }
    })

  /** Při chybě ukáže toast a chybu znovu vyhodí (dialog hledání zůstane otevřený). */
  const match = (t: BankTransaction, target: MatchTarget, key = 'match') =>
    track(t.id, key, async () => {
      try {
        const matched = await matchM.mutateAsync({ id: t.id, body: target.body })
        toast.success(`Spárováno: ${target.label}`, {
          description: 'Úhrada byla zapsána k dokladu.',
          action: { label: 'Vrátit', onClick: () => void unmatch(matched, { silent: true }) },
        })
      } catch (err) {
        toast.error(matchErrorText(err))
        throw err
      }
    })

  const setIgnored = (t: BankTransaction, ignored: boolean) =>
    track(t.id, ignored ? 'ignore' : 'unignore', async () => {
      try {
        await ignoreM.mutateAsync({ id: t.id, ignored })
        if (ignored) {
          toast('Platba ignorována', {
            action: { label: 'Vrátit', onClick: () => void setIgnored(t, false) },
          })
        }
      } catch (err) {
        toast.error(matchErrorText(err))
      }
    })

  return { match, unmatch, setIgnored, busy }
}

export type TransactionActions = ReturnType<typeof useTransactionActions>

export function suggestionTarget(s: MatchSuggestion): MatchTarget {
  return s.invoice_id
    ? { body: { invoice_id: s.invoice_id }, label: `faktura ${s.number}` }
    : { body: { expense_id: s.expense_id }, label: `náklad ${s.number}` }
}
