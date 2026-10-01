import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateCorrection } from '@/api/queries/invoices'
import type { Invoice } from '@/api/types'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { CORRECTION_REASONS } from './form-model'

function makeSchema(required: boolean) {
  return z.object({
    correction_reason: z
      .string()
      .trim()
      .max(500, 'Nejvýše 500 znaků')
      .refine((v) => !required || v !== '', 'Uveďte důvod opravy'),
  })
}
type Values = z.infer<ReturnType<typeof makeSchema>>

/** Dialog „Vystavit opravný doklad“: důvod opravy (§ 45 ZDPH, u plátce povinný) a přechod na úpravu dobropisu. */
export function CorrectionDialog({
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
  const create = useCreateCorrection(slug, invoice.id)
  const required = invoice.your_vat_mode === 'vat_payer'
  const schema = useMemo(() => makeSchema(required), [required])
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { correction_reason: '' } })

  useEffect(() => {
    if (open) form.reset({ correction_reason: '' })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset jen při otevření
  }, [open])

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      const c = await create.mutateAsync(v.correction_reason ? { correction_reason: v.correction_reason } : {})
      onOpenChange(false)
      toast.success(`Opravný doklad ${c.number} vystaven`)
      void navigate({ to: '/a/$slug/invoices/$invoiceId/edit', params: { slug, invoiceId: c.id } })
    } catch (err) {
      if (!applyProblemToForm(err, form.setError, ['correction_reason'])) toast.error(errorMessage(err))
    }
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !create.isPending && onOpenChange(o)}
      title="Vystavit opravný doklad"
      description={`Vznikne opravný daňový doklad k ${invoice.number} se zápornými množstvími. Pak ho můžete upravit.`}
      footer={
        <>
          <Button type="submit" form="correction-form" disabled={create.isPending}>
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
        id="correction-form"
        noValidate
        onSubmit={(e) => {
          e.stopPropagation()
          void onSubmit(e)
        }}
        className="flex flex-col gap-3"
      >
        <TextField
          control={form.control}
          name="correction_reason"
          label={required ? 'Důvod opravy' : 'Důvod opravy (nepovinné)'}
          autoComplete="off"
          maxLength={500}
          description={required ? 'Plátce DPH musí na opravném dokladu uvést důvod opravy.' : undefined}
        />
        <div className="flex flex-wrap gap-1.5" role="group" aria-label="Návrhy důvodu">
          {CORRECTION_REASONS.map((r) => (
            <Button
              key={r}
              type="button"
              variant="outline"
              size="sm"
              onClick={() => form.setValue('correction_reason', r, { shouldValidate: form.formState.isSubmitted })}
            >
              {r}
            </Button>
          ))}
        </div>
      </form>
    </ResponsiveDialog>
  )
}
