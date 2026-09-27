import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreatePayment } from '@/api/queries/invoices'
import type { Invoice } from '@/api/types'
import { SwitchField, TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { parseISODate, todayISO } from '@/lib/date'
import { formatMoney, formatMoneyInput, parseMoney } from '@/lib/money'

const schema = z.object({
  paid_on: z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum'),
  amount: z.string().refine((v) => {
    const n = parseMoney(v)
    return n !== null && n !== 0
  }, 'Zadejte nenulovou částku'),
  note: z.string().max(500),
  create_final_invoice: z.boolean(),
})
type Values = z.infer<typeof schema>

function defaults(inv: Invoice): Values {
  return { paid_on: todayISO(), amount: formatMoneyInput(inv.remaining_amount), note: '', create_final_invoice: inv.document_type === 'proforma' }
}

export function AddPaymentDialog({
  slug,
  invoice,
  open,
  onOpenChange,
}: {
  slug: string
  invoice: Invoice
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const navigate = useNavigate()
  const create = useCreatePayment(slug, invoice.id)
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: defaults(invoice) })

  useEffect(() => {
    if (open) form.reset(defaults(invoice))
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset jen při otevření
  }, [open])

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      const res = await create.mutateAsync({
        paid_on: v.paid_on,
        amount: parseMoney(v.amount) ?? undefined,
        note: v.note.trim() || undefined,
        create_final_invoice: invoice.document_type === 'proforma' ? v.create_final_invoice : undefined,
      })
      onOpenChange(false)
      if (res.final_invoice_id) {
        toast.success('Platba přidána a vystavena konečná faktura')
        void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: res.final_invoice_id } })
      } else {
        toast.success(res.invoice.status === 'paid' ? 'Faktura je uhrazená' : 'Platba přidána')
      }
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Přidat platbu"
      description={`${invoice.number} · zbývá ${formatMoney(invoice.remaining_amount, invoice.currency)}`}
      footer={
        <>
          <Button type="submit" form="add-payment-form" disabled={create.isPending}>
            {create.isPending && <Spinner data-icon="inline-start" />}
            Přidat platbu
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      <form
        id="add-payment-form"
        noValidate
        onSubmit={(e) => {
          e.stopPropagation()
          void onSubmit(e)
        }}
        className="flex flex-col gap-4"
      >
        <div className="grid grid-cols-2 gap-4">
          <TextField control={form.control} name="paid_on" label="Datum úhrady" type="date" />
          <TextField
            control={form.control}
            name="amount"
            label={`Částka (${invoice.currency})`}
            inputMode="decimal"
            autoComplete="off"
            enterKeyHint="done"
          />
        </div>
        <TextField control={form.control} name="note" label="Poznámka" autoComplete="off" placeholder="Nepovinné" />
        {invoice.document_type === 'proforma' && (
          <SwitchField
            control={form.control}
            name="create_final_invoice"
            label="Vystavit konečnou fakturu"
            description="Vytvoří uhrazenou fakturu se stejnými položkami."
          />
        )}
      </form>
    </ResponsiveDialog>
  )
}
