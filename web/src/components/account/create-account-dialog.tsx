import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateAccount } from '@/api/queries/accounts'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

const schema = z.object({
  name: z.string().trim().min(1, 'Zadejte název'),
})
type Values = z.infer<typeof schema>

/** Založení dalšího účtu (firmy / OSVČ). Po vytvoření přepne na nový účet. */
export function CreateAccountDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const create = useCreateAccount()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: '' } })

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const account = await create.mutateAsync(values)
      toast.success(`Účet „${account.name}“ založen`)
      onOpenChange(false)
      form.reset()
      await navigate({ to: '/a/$slug/settings/company', params: { slug: account.slug } })
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Nový účet"
      description="Další firma nebo OSVČ, za kterou budete vystavovat doklady."
      footer={
        <>
          <Button type="submit" form="create-account-form" disabled={create.isPending}>
            {create.isPending && <Spinner data-icon="inline-start" />}
            Založit účet
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id="create-account-form" onSubmit={onSubmit} noValidate>
        <TextField
          control={form.control}
          name="name"
          label="Název firmy / jméno"
          autoComplete="organization"
          enterKeyHint="done"
        />
      </form>
    </ResponsiveDialog>
  )
}
