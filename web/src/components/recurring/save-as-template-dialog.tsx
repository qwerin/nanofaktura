import { useNavigate } from '@tanstack/react-router'
import { useId, useState } from 'react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/errors'
import { useSaveAsTemplate } from '@/api/queries/templates'
import type { Invoice } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'

/** „Uložit jako šablonu“ z detailu faktury → POST /invoices/{id}/save-as-template. */
export function SaveAsTemplateDialog({
  slug,
  invoice,
  open,
  onOpenChange,
}: {
  slug: string
  invoice: Invoice
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const save = useSaveAsTemplate(slug, invoice.id)
  const defaultName = [invoice.client_name, invoice.number].filter(Boolean).join(' ')
  const [name, setName] = useState('')
  const id = useId()

  const submit = () =>
    save.mutate(
      { name: name.trim() || undefined },
      {
        onSuccess: (t) => {
          onOpenChange(false)
          setName('')
          toast.success(`Šablona „${t.name}“ uložena`, {
            action: {
              label: 'Nastavit opakování',
              onClick: () => void navigate({ to: '/a/$slug/recurring/new', params: { slug }, search: { template_id: t.id } }),
            },
          })
        },
        onError: (err) => toast.error(errorMessage(err)),
      },
    )

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !save.isPending && onOpenChange(o)}
      title="Uložit jako šablonu"
      description="Odběratel, položky, měna, platba a texty se uloží pro další faktury nebo pravidelné vystavování."
      footer={
        <>
          <Button onClick={submit} disabled={save.isPending}>
            {save.isPending && <Spinner data-icon="inline-start" />}
            Uložit šablonu
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={save.isPending}>
            Zrušit
          </Button>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <Field>
          <FieldLabel htmlFor={id}>Název šablony</FieldLabel>
          <Input
            id={id}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={defaultName}
            autoComplete="off"
            enterKeyHint="done"
            maxLength={200}
          />
          <FieldDescription>
            Tip: v šabloně můžete v názvech položek použít např. {'{MONTH_NAME}'} — doplní se měsíc vystavení.
          </FieldDescription>
        </Field>
      </form>
    </ResponsiveDialog>
  )
}
