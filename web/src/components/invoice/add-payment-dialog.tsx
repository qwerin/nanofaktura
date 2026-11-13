import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ReceiptTextIcon } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { eetQueries } from '@/api/queries/eet'
import { useCreatePayment } from '@/api/queries/invoices'
import type { EETSale, Invoice } from '@/api/types'
import { SwitchField, TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { parseISODate, todayISO } from '@/lib/date'
import { defaultContactPayment } from '@/lib/eet'
import { formatMoney, formatMoneyInput, parseMoney } from '@/lib/money'
import { paymentAmountError, precheckFinalInvoice, remainderOnFinalInvoice } from './payment-logic'

function makeSchema(remaining: number) {
  return z.object({
    paid_on: z.string().refine((v) => parseISODate(v) !== null, 'Zadejte datum'),
    amount: z.string().superRefine((v, ctx) => {
      const msg = paymentAmountError(parseMoney(v), remaining)
      if (msg) ctx.addIssue({ code: 'custom', message: msg })
    }),
    note: z.string().max(500),
    create_final_invoice: z.boolean(),
    eet: z.boolean(),
  })
}
type Values = z.infer<ReturnType<typeof makeSchema>>

function defaults(inv: Invoice): Values {
  return {
    paid_on: todayISO(),
    amount: formatMoneyInput(inv.remaining_amount),
    note: '',
    create_final_invoice: inv.document_type === 'proforma' && precheckFinalInvoice(inv.remaining_amount, inv.remaining_amount),
    eet: defaultContactPayment(inv.payment_method),
  }
}

/** Plátce bez přenesení daňové povinnosti: k platbě zálohy vzniká daňový doklad (§ 28 ZDPH). */
function issuesTaxDocument(inv: Invoice): boolean {
  return inv.document_type === 'proforma' && inv.your_vat_mode === 'vat_payer' && !inv.reverse_charge
}

/** Tržba v EET, která vznikla k právě přidané platbě (odpověď už obsahuje výsledek pokusu o odeslání). */
function saleOfPayment(invoice: Invoice, paymentId: number): EETSale | undefined {
  return (invoice.eet_sales ?? []).find((s) => s.payment_id === paymentId && s.cancels_id == null)
}

/** Neblokující upozornění, když se tržbu nepodařilo hned zaevidovat. */
function notifyEetOutcome(sale: EETSale | undefined) {
  if (!sale || sale.status === 'confirmed') return
  if (sale.status === 'failed') {
    toast.error('Platbu se nepodařilo zaevidovat do EET', {
      description: sale.error || 'Finanční správa tržbu odmítla. Podrobnosti najdete u platby na faktuře.',
      duration: 10_000,
    })
  } else if (sale.status === 'pending') {
    toast.warning('Tržba čeká na odeslání do EET', {
      description: 'Finanční správa teď neodpovídá — odešle se automaticky.',
    })
  }
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
  // Volba kontaktní platby se nabízí jen při zapnuté evidenci tržeb (načte se už s detailem faktury);
  // dokud nastavení neznáme, pole neposíláme a o evidenci rozhodne výchozí pravidlo serveru.
  const eet = useQuery(eetQueries.settings(slug))
  const eetEnabled = eet.data?.enabled === true
  const schema = useMemo(() => makeSchema(invoice.remaining_amount), [invoice.remaining_amount])
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: defaults(invoice) })
  const proforma = invoice.document_type === 'proforma'
  const [amountText, finalChecked] = useWatch({ control: form.control, name: ['amount', 'create_final_invoice'] })
  const amount = parseMoney(amountText ?? '')
  const full = precheckFinalInvoice(amount, invoice.remaining_amount)
  const rest = remainderOnFinalInvoice(amount, invoice.remaining_amount)

  useEffect(() => {
    if (open) form.reset(defaults(invoice))
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset jen při otevření
  }, [open])

  // Vyúčtování se předvolí jen při úhradě celé zbývající částky; změna částky volbu přepne.
  useEffect(() => {
    if (proforma && open) form.setValue('create_final_invoice', full)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- jen při přechodu plná ↔ částečná platba
  }, [full])

  const openInvoice = (id: number) => void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: id } })

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      const res = await create.mutateAsync({
        paid_on: v.paid_on,
        amount: parseMoney(v.amount) ?? undefined,
        note: v.note.trim() || undefined,
        create_final_invoice: proforma ? v.create_final_invoice : undefined,
        // Při zapnuté evidenci vždy výslovně; jinak pole neposíláme (true by skončilo chybou).
        eet: eetEnabled ? v.eet : undefined,
      })
      onOpenChange(false)
      notifyEetOutcome(saleOfPayment(res.invoice, res.payment.id))
      const taxDocumentId = res.tax_document_id
      const taxDocAction = taxDocumentId
        ? { action: { label: 'Zobrazit daňový doklad', onClick: () => openInvoice(taxDocumentId) } }
        : undefined
      if (res.final_invoice_id) {
        toast.success(
          taxDocumentId ? 'Platba přidána, vystaven daňový doklad k platbě a konečná faktura' : 'Platba přidána a vystavena konečná faktura',
          taxDocAction,
        )
        openInvoice(res.final_invoice_id)
      } else if (taxDocumentId) {
        toast.success('Platba přidána a vystaven daňový doklad k přijaté platbě', taxDocAction)
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
        {eetEnabled && (
          <SwitchField
            control={form.control}
            name="eet"
            label="Kontaktní platba — zaevidovat do EET"
            description={
              eet.data?.environment === 'playground'
                ? 'Hotově nebo kartou na místě; převod na účet se neeviduje. Běží testovací prostředí — potvrzení nebude platné.'
                : 'Hotově nebo kartou na místě; převod na účet se neeviduje.'
            }
          />
        )}
        {proforma && issuesTaxDocument(invoice) && (
          <p className="flex gap-2 rounded-lg bg-muted/60 p-3 text-sm text-muted-foreground">
            <ReceiptTextIcon className="mt-0.5 size-4 shrink-0" />
            <span>K přijaté platbě se automaticky vystaví daňový doklad (z platby zálohy odvádíte DPH).</span>
          </p>
        )}
        {proforma && (
          <SwitchField
            control={form.control}
            name="create_final_invoice"
            label="Vystavit konečnou fakturu"
            description={
              finalChecked && rest !== 0
                ? `Částečná platba: konečná faktura převezme platby zálohy a zbylých ${formatMoney(rest, invoice.currency)} zůstane k úhradě na ní. Záloha se tím uzavře.`
                : 'Vytvoří fakturu se stejnými položkami, převezme platby zálohy a zálohu uzavře.'
            }
          />
        )}
      </form>
    </ResponsiveDialog>
  )
}
