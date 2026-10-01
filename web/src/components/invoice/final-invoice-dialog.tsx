import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateFinalInvoice } from '@/api/queries/invoices'
import type { Invoice } from '@/api/types'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { parseISODate, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { requiresTaxableDate } from './form-model'

const dateString = z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum')
const schema = z.object({ issued_on: dateString, taxable_fulfillment_due: z.string() })
type Values = z.infer<typeof schema>

/** Dialog „Vystavit vyúčtování“ zálohové faktury: datum vystavení a (u plátce) DUZP = datum dodání. */
export function FinalInvoiceDialog({
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
  const create = useCreateFinalInvoice(slug, invoice.id)
  const withDuzp = requiresTaxableDate(invoice.your_vat_mode, invoice.reverse_charge)
  const form = useForm<Values>({
    resolver: zodResolver(
      schema.superRefine((v, ctx) => {
        if (withDuzp && !parseISODate(v.taxable_fulfillment_due)) {
          ctx.addIssue({ code: 'custom', path: ['taxable_fulfillment_due'], message: 'Zadejte datum zdanitelného plnění' })
        }
      }),
    ),
    defaultValues: { issued_on: todayISO(), taxable_fulfillment_due: todayISO() },
  })

  useEffect(() => {
    if (open) form.reset({ issued_on: todayISO(), taxable_fulfillment_due: todayISO() })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset jen při otevření
  }, [open])

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      const inv = await create.mutateAsync({
        issued_on: v.issued_on,
        ...(withDuzp ? { taxable_fulfillment_due: v.taxable_fulfillment_due } : {}),
      })
      onOpenChange(false)
      toast.success(`Vystaveno vyúčtování ${inv.number}`)
      void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: inv.id } })
    } catch (err) {
      if (!applyProblemToForm(err, form.setError, ['issued_on', 'taxable_fulfillment_due'])) toast.error(errorMessage(err))
    }
  })

  const paid = invoice.paid_amount !== 0
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !create.isPending && onOpenChange(o)}
      title="Vystavit vyúčtování"
      description={
        paid
          ? `Vznikne konečná faktura se stejnými položkami. Převezme platby zálohy (${formatMoney(invoice.paid_amount, invoice.currency)}) a záloha se uzavře.`
          : 'Vznikne konečná faktura se stejnými položkami a záloha se uzavře.'
      }
      footer={
        <>
          <Button type="submit" form="final-invoice-form" disabled={create.isPending}>
            {create.isPending && <Spinner data-icon="inline-start" />}
            Vystavit
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={create.isPending}>
            Zrušit
          </Button>
        </>
      }
    >
      <form
        id="final-invoice-form"
        noValidate
        onSubmit={(e) => {
          e.stopPropagation()
          void onSubmit(e)
        }}
        className="grid gap-4 sm:grid-cols-2"
      >
        <Controller
          control={form.control}
          name="issued_on"
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid || undefined}>
              <FieldLabel htmlFor="final-invoice-issued-on">Datum vystavení</FieldLabel>
              <Input
                id="final-invoice-issued-on"
                type="date"
                {...field}
                aria-invalid={fieldState.invalid || undefined}
                onChange={(e) => {
                  // DUZP drží krok s datem vystavení, dokud ho uživatel nezmění.
                  if (form.getValues('taxable_fulfillment_due') === field.value) {
                    form.setValue('taxable_fulfillment_due', e.target.value)
                  }
                  field.onChange(e.target.value)
                }}
              />
              <FieldError errors={[fieldState.error]} />
            </Field>
          )}
        />
        {withDuzp && (
          <TextField
            control={form.control}
            name="taxable_fulfillment_due"
            label="Datum zdanitelného plnění"
            type="date"
            description="Den dodání zboží nebo služby."
          />
        )}
      </form>
    </ResponsiveDialog>
  )
}
