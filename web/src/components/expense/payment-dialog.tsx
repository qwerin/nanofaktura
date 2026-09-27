import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateExpensePayment } from '@/api/queries/expenses'
import type { Expense } from '@/api/types'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { parseISODate, todayISO } from '@/lib/date'
import { formatMoney, formatMoneyInput, parseMoney } from '@/lib/money'

const schema = z.object({
  paid_on: z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum'),
  amount: z.string().refine((v) => {
    const m = parseMoney(v)
    return m !== null && m !== 0
  }, 'Zadejte nenulovou částku'),
  note: z.string().max(500, 'Nejvýše 500 znaků'),
})
type Values = z.infer<typeof schema>

/** Přidání úhrady nákladu (předvyplní zbývající částku a dnešek). */
export function ExpensePaymentDialog({
  expense,
  slug,
  open,
  onOpenChange,
}: {
  expense: Expense
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Přidat úhradu"
      description={`Zbývá uhradit ${formatMoney(expense.remaining_amount, expense.currency)}.`}
      footer={
        <>
          <Button type="submit" form="expense-payment-form">
            Uložit úhradu
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      {open && <PaymentForm expense={expense} slug={slug} onDone={() => onOpenChange(false)} />}
    </ResponsiveDialog>
  )
}

function PaymentForm({ expense, slug, onDone }: { expense: Expense; slug: string; onDone: () => void }) {
  const create = useCreateExpensePayment(slug, expense.id)
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      paid_on: todayISO(),
      amount: expense.remaining_amount > 0 ? formatMoneyInput(expense.remaining_amount) : '',
      note: '',
    },
  })

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      await create.mutateAsync({ paid_on: v.paid_on, amount: parseMoney(v.amount) ?? 0, note: v.note.trim() })
      toast.success('Úhrada přidána')
      onDone()
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <form id="expense-payment-form" onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      <div className="grid grid-cols-2 gap-3">
        <TextField control={form.control} name="paid_on" label="Datum úhrady" type="date" />
        <TextField control={form.control} name="amount" label={`Částka (${expense.currency})`} inputMode="decimal" autoComplete="off" enterKeyHint="next" />
      </div>
      <TextField control={form.control} name="note" label="Poznámka" autoComplete="off" placeholder="nepovinné" enterKeyHint="done" />
      {create.isPending && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner /> Ukládám…
        </p>
      )}
    </form>
  )
}
