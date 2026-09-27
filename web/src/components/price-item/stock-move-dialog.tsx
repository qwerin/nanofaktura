import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateStockMove } from '@/api/queries/price-items'
import type { PriceItem } from '@/api/types'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { parseISODate, todayISO } from '@/lib/date'
import { parseQuantity } from '@/lib/quantity'
import { formatQuantity } from '@/lib/quantity'

const schema = z.object({
  quantity: z.string().refine((v) => {
    const q = parseQuantity(v)
    return q !== null && q !== '0' && !q.startsWith('-')
  }, 'Zadejte kladné množství'),
  moved_on: z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum'),
  note: z.string().max(500, 'Nejvýše 500 znaků'),
})
type Values = z.infer<typeof schema>

/** Ruční příjem / výdej ze skladu. */
export function StockMoveDialog({
  item,
  slug,
  direction,
  onClose,
}: {
  item: PriceItem
  slug: string
  /** `null` = zavřeno. */
  direction: 'in' | 'out' | null
  onClose: () => void
}) {
  return (
    <ResponsiveDialog
      open={direction !== null}
      onOpenChange={(o) => !o && onClose()}
      title={direction === 'out' ? 'Výdej ze skladu' : 'Příjem na sklad'}
      description={`${item.name} · aktuálně ${formatQuantity(item.stock_quantity)} ${item.unit_name}`}
      footer={
        <>
          <Button type="submit" form="stock-move-form">
            {direction === 'out' ? 'Vydat' : 'Přijmout'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            Zrušit
          </Button>
        </>
      }
    >
      {direction && <MoveForm key={direction} item={item} slug={slug} direction={direction} onDone={onClose} />}
    </ResponsiveDialog>
  )
}

function MoveForm({ item, slug, direction, onDone }: { item: PriceItem; slug: string; direction: 'in' | 'out'; onDone: () => void }) {
  const create = useCreateStockMove(slug, item.id)
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { quantity: '', moved_on: todayISO(), note: '' } })

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      await create.mutateAsync({ direction, quantity: parseQuantity(v.quantity) ?? '0', moved_on: v.moved_on, note: v.note.trim() })
      toast.success(direction === 'in' ? 'Příjem zapsán' : 'Výdej zapsán')
      onDone()
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <form id="stock-move-form" onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      <div className="grid grid-cols-2 gap-3">
        <TextField control={form.control} name="quantity" label={`Množství${item.unit_name ? ` (${item.unit_name})` : ''}`} inputMode="decimal" autoComplete="off" autoFocus enterKeyHint="next" />
        <TextField control={form.control} name="moved_on" label="Datum" type="date" />
      </div>
      <TextField
        control={form.control}
        name="note"
        label="Poznámka"
        autoComplete="off"
        placeholder={direction === 'in' ? 'např. dodávka od výrobce' : 'např. inventura, poškození'}
        enterKeyHint="done"
      />
      {create.isPending && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner /> Ukládám…
        </p>
      )}
    </form>
  )
}
