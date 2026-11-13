import { BanIcon, CheckIcon, CircleAlertIcon, ClockIcon, CopyIcon, RotateCwIcon, type LucideIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/errors'
import { useRetryEETSale } from '@/api/queries/eet'
import type { EETSale, EETSaleStatus } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { eetStatusLabels, eetStatusTones, isCancellation, type EetTone } from '@/lib/eet'
import { cn } from '@/lib/utils'

const toneClasses: Record<EetTone, string> = {
  success: 'bg-success/12 text-success dark:bg-success/20',
  warning: 'bg-warning/15 text-warning-foreground dark:text-warning',
  destructive: 'bg-destructive/10 text-destructive dark:bg-destructive/20',
  muted: 'bg-muted text-muted-foreground',
}

const statusIcons: Record<EETSaleStatus, LucideIcon> = {
  confirmed: CheckIcon,
  pending: ClockIcon,
  failed: CircleAlertIcon,
  cancelled: BanIcon,
}

/** Štítek stavu tržby v EET (ikona + text, nikdy jen barva). `prefix` přidá „EET:“ tam, kde chybí kontext. */
export function EetStatusBadge({ status, prefix, className }: { status: EETSaleStatus; prefix?: boolean; className?: string }) {
  const Icon = statusIcons[status]
  return (
    <Badge variant="secondary" className={cn('gap-1', toneClasses[eetStatusTones[status]], className)}>
      <Icon data-icon="inline-start" aria-hidden="true" />
      {prefix && 'EET: '}
      {eetStatusLabels[status]}
    </Badge>
  )
}

/** Štítky navíc: storno a testovací prostředí (potvrzení z něj nejsou platná). */
export function EetSaleFlags({
  sale,
  hideCancellation,
}: {
  sale: Pick<EETSale, 'cancels_id' | 'amount' | 'environment'>
  hideCancellation?: boolean
}) {
  return (
    <>
      {!hideCancellation && isCancellation(sale) && <Badge variant="outline">Storno</Badge>}
      {sale.environment === 'playground' && (
        <Badge variant="outline" title="Testovací prostředí Finanční správy — potvrzení není platné">
          Test
        </Badge>
      )}
    </>
  )
}

/** Potvrzovací kód (POK): malé neproporcionální písmo, zalamuje se, jde zkopírovat. */
export function EetPok({ pok, className }: { pok: string; className?: string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(t)
  }, [copied])

  return (
    <span className={cn('flex min-w-0 items-center gap-1', className)}>
      <span className="min-w-0 text-xs text-muted-foreground">
        POK <span className="font-mono break-all text-foreground">{pok}</span>
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        className="shrink-0 text-muted-foreground max-md:size-9"
        aria-label={copied ? 'Zkopírováno' : 'Kopírovat potvrzovací kód'}
        onClick={async (e) => {
          e.stopPropagation()
          try {
            await navigator.clipboard.writeText(pok)
            setCopied(true)
          } catch {
            toast.error('Kopírování se nezdařilo — označte text a zkopírujte ho ručně.')
          }
        }}
      >
        {copied ? <CheckIcon className="text-success" /> : <CopyIcon />}
      </Button>
    </span>
  )
}

/** „Zkusit znovu“ pro čekající/odmítnutou tržbu; výsledek oznámí toastem. */
export function EetRetryButton({ slug, sale, className }: { slug: string; sale: Pick<EETSale, 'id' | 'invoice_id'>; className?: string }) {
  const retry = useRetryEETSale(slug)
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className={cn('max-md:h-10', className)}
      disabled={retry.isPending}
      onClick={() =>
        retry.mutate(sale, {
          onSuccess: (s) => {
            if (s.status === 'confirmed') toast.success('Tržba je zaevidovaná')
            else if (s.status === 'failed') toast.error(s.error ? `Finanční správa tržbu odmítla: ${s.error}` : 'Finanční správa tržbu odmítla')
            else toast.info('Odeslání se zatím nepovedlo — aplikace to zkusí sama znovu.')
          },
          onError: (err) => toast.error(errorMessage(err)),
        })
      }
    >
      {retry.isPending ? <Spinner data-icon="inline-start" /> : <RotateCwIcon data-icon="inline-start" />}
      Zkusit znovu
    </Button>
  )
}
