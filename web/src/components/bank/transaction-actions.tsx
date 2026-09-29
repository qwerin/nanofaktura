import { BanIcon, FileTextIcon, LinkIcon, ReceiptIcon, RotateCcwIcon, SearchIcon, UnlinkIcon } from 'lucide-react'
import type { BankTransaction, MatchSuggestion } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { suggestionConfidence } from './format'
import { MatchedDocumentLink, ReasonChips } from './transaction-parts'
import { suggestionTarget, type TransactionActions } from './use-transaction-actions'

function suggestionKey(s: MatchSuggestion) {
  return s.invoice_id ? `i${s.invoice_id}` : `e${s.expense_id}`
}

/** Návrhy dokladů u nespárované platby s tlačítkem „Spárovat“ (jedním klepnutím). */
export function SuggestionList({
  transaction: t,
  actions,
  canEdit,
}: {
  transaction: BankTransaction
  actions: TransactionActions
  canEdit: boolean
}) {
  const busy = actions.busy[t.id]
  return (
    <ul className="flex flex-col gap-2">
      {t.suggestions.map((s, i) => {
        const key = `match-${suggestionKey(s)}`
        const confidence = suggestionConfidence(s.score)
        const Icon = s.invoice_id ? FileTextIcon : ReceiptIcon
        return (
          <li
            key={suggestionKey(s)}
            className={cn(
              'flex flex-col gap-2 rounded-lg border bg-background p-3 sm:flex-row sm:items-center sm:gap-3',
              i === 0 && confidence === 'high' && 'border-success/40',
            )}
          >
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              <div className="flex min-w-0 items-baseline gap-2">
                <Icon className="size-4 shrink-0 translate-y-0.5 text-muted-foreground" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate text-sm">
                  <span className="font-medium">{s.number}</span>
                  {s.name && <span className="text-muted-foreground"> · {s.name}</span>}
                </span>
              </div>
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 pl-6">
                <span className="text-xs tabular-nums text-muted-foreground">zbývá {formatMoney(s.remaining, t.currency)}</span>
                <ReasonChips reasons={s.reasons} />
              </div>
            </div>
            {canEdit && (
              <Button
                size="sm"
                variant={i === 0 ? 'default' : 'outline'}
                className="h-11 shrink-0 md:h-8"
                disabled={Boolean(busy)}
                onClick={() => void actions.match(t, suggestionTarget(s), key).catch(() => undefined)}
              >
                {busy === key ? <Spinner data-icon="inline-start" /> : <LinkIcon data-icon="inline-start" />}
                Spárovat
              </Button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

/**
 * Párovací část řádku/karty podle stavu: návrhy + „Hledat doklad…“ + „Ignorovat“,
 * u spárované odkaz na doklad + „Zrušit spárování“, u ignorované „Obnovit“.
 */
export function TransactionMatchPanel({
  slug,
  transaction: t,
  actions,
  canEdit,
  onSearch,
  onUnmatch,
}: {
  slug: string
  transaction: BankTransaction
  actions: TransactionActions
  canEdit: boolean
  onSearch: (t: BankTransaction) => void
  onUnmatch: (t: BankTransaction) => void
}) {
  const busy = actions.busy[t.id]

  if (t.state === 'matched') {
    return (
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <MatchedDocumentLink slug={slug} transaction={t} className="min-w-0 flex-1" />
        {canEdit && (
          <Button variant="ghost" size="sm" className="h-11 text-muted-foreground md:h-8" disabled={Boolean(busy)} onClick={() => onUnmatch(t)}>
            {busy === 'unmatch' ? <Spinner data-icon="inline-start" /> : <UnlinkIcon data-icon="inline-start" />}
            Zrušit spárování
          </Button>
        )}
      </div>
    )
  }

  if (t.state === 'ignored') {
    return canEdit ? (
      <div className="flex items-center gap-2">
        <span className="flex-1 text-sm text-muted-foreground">Nezapočítává se do párování.</span>
        <Button variant="outline" size="sm" className="h-11 md:h-8" disabled={Boolean(busy)} onClick={() => void actions.setIgnored(t, false)}>
          {busy === 'unignore' ? <Spinner data-icon="inline-start" /> : <RotateCcwIcon data-icon="inline-start" />}
          Obnovit
        </Button>
      </div>
    ) : null
  }

  return (
    <div className="flex flex-col gap-2">
      {t.suggestions.length > 0 && <SuggestionList transaction={t} actions={actions} canEdit={canEdit} />}
      {canEdit && (
        <div className="flex gap-2">
          <Button variant="outline" size="sm" className="h-11 flex-1 md:h-8 sm:flex-none" disabled={Boolean(busy)} onClick={() => onSearch(t)}>
            <SearchIcon data-icon="inline-start" />
            Hledat doklad…
          </Button>
          <Button variant="ghost" size="sm" className="h-11 text-muted-foreground md:h-8" disabled={Boolean(busy)} onClick={() => void actions.setIgnored(t, true)}>
            {busy === 'ignore' ? <Spinner data-icon="inline-start" /> : <BanIcon data-icon="inline-start" />}
            Ignorovat
          </Button>
        </div>
      )}
    </div>
  )
}
