import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { SearchIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, unwrap } from '@/api/client'
import { applyProblemToForm, errorMessage, isApiError } from '@/api/errors'
import { useCreateSubject } from '@/api/queries/subjects'
import type { CreateSubjectInput as SubjectCreate } from '@/api/types'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { isValidIco, optionalEmailSchema, optionalIcoSchema } from '@/lib/validation'
import type { PickerSubject } from './subject-picker'

const schema = z.object({
  registration_no: optionalIcoSchema,
  name: z.string().trim().min(1, 'Zadejte název'),
  vat_no: z.string().trim(),
  street: z.string(),
  city: z.string(),
  zip: z.string().trim().max(10, 'Nejvýše 10 znaků'),
  country: z.string().trim().regex(/^[A-Za-z]{2}$/, 'Kód země, např. CZ'),
  email: optionalEmailSchema,
})
type Values = z.infer<typeof schema>

const empty: Values = { registration_no: '', name: '', vat_no: '', street: '', city: '', zip: '', country: 'CZ', email: '' }

interface Props {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (subject: PickerSubject) => void
}

/** Rychlé založení kontaktu z formuláře faktury, s předvyplněním z ARES. */
export function NewSubjectDialog({ slug, open, onOpenChange, onCreated }: Props) {
  const icoId = useId()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: empty })
  const [aresNote, setAresNote] = useState<string | null>(null)

  const create = useCreateSubject(slug)

  const ares = useMutation({
    mutationFn: (ico: string) => unwrap(api.GET('/api/ares/{ico}', { params: { path: { ico } } })),
    meta: { silent: true },
  })

  const lookup = async () => {
    const ico = form.getValues('registration_no').trim()
    if (!isValidIco(ico)) {
      form.setError('registration_no', { message: 'Zadejte platné IČO' })
      return
    }
    setAresNote(null)
    try {
      const r = await ares.mutateAsync(ico)
      const opts = { shouldDirty: true, shouldValidate: true }
      form.setValue('registration_no', r.registration_no, opts)
      form.setValue('name', r.name, opts)
      form.setValue('vat_no', r.vat_no, opts)
      form.setValue('street', r.street, opts)
      form.setValue('city', r.city, opts)
      form.setValue('zip', r.zip, opts)
      form.setValue('country', r.country || 'CZ', opts)
      setAresNote(`Načteno z ARES: ${r.name}`)
    } catch (err) {
      if (isApiError(err) && err.status === 404) form.setError('registration_no', { message: 'Subjekt s tímto IČO v ARES není.' })
      else if (isApiError(err) && err.status === 502) toast.error('ARES je teď nedostupný, vyplňte údaje ručně.')
      else form.setError('registration_no', { message: errorMessage(err) })
    }
  }

  const close = (next: boolean) => {
    onOpenChange(next)
    if (!next) {
      setTimeout(() => {
        form.reset(empty)
        setAresNote(null)
      }, 300)
    }
  }

  const onSubmit = form.handleSubmit(async (v) => {
    try {
      const body: SubjectCreate = {
        name: v.name.trim(),
        registration_no: v.registration_no || undefined,
        vat_no: v.vat_no.toUpperCase() || undefined,
        street: v.street || undefined,
        city: v.city || undefined,
        zip: v.zip || undefined,
        country: v.country.toUpperCase(),
        email: v.email || undefined,
      }
      const s = await create.mutateAsync(body)
      toast.success(`Kontakt ${s.name} založen`)
      onCreated(s)
      close(false)
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={close}
      title="Nový kontakt"
      description="Zadejte IČO a údaje doplníme z ARES."
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="submit" form="new-subject-form" disabled={create.isPending}>
            {create.isPending && <Spinner data-icon="inline-start" />}
            Založit a vybrat
          </Button>
          <Button variant="outline" onClick={() => close(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      <form
        id="new-subject-form"
        // Dialog je v portálu, ale React události probublávají i do formuláře faktury.
        onSubmit={(e) => {
          e.stopPropagation()
          void onSubmit(e)
        }}
        noValidate
        className="flex flex-col gap-4"
      >
        <Controller
          control={form.control}
          name="registration_no"
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid || undefined}>
              <FieldLabel htmlFor={icoId}>IČO</FieldLabel>
              <div className="flex gap-2">
                <Input
                  id={icoId}
                  {...field}
                  inputMode="numeric"
                  autoComplete="off"
                  maxLength={8}
                  placeholder="12345678"
                  enterKeyHint="search"
                  aria-invalid={fieldState.invalid || undefined}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      void lookup()
                    }
                  }}
                />
                <Button type="button" variant="secondary" onClick={() => void lookup()} disabled={ares.isPending} className="shrink-0">
                  {ares.isPending ? <Spinner data-icon="inline-start" /> : <SearchIcon data-icon="inline-start" />}
                  ARES
                </Button>
              </div>
              {aresNote && !fieldState.invalid && <FieldDescription className="text-success">{aresNote}</FieldDescription>}
              <FieldError errors={[fieldState.error]} />
            </Field>
          )}
        />
        <TextField control={form.control} name="name" label="Název / jméno" autoComplete="organization" />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField control={form.control} name="vat_no" label="DIČ" autoCapitalize="characters" spellCheck={false} autoComplete="off" />
          <TextField
            control={form.control}
            name="email"
            label="E-mail"
            type="email"
            inputMode="email"
            autoComplete="email"
            autoCapitalize="none"
            spellCheck={false}
          />
        </div>
        <TextField control={form.control} name="street" label="Ulice a číslo" autoComplete="street-address" />
        <div className="grid grid-cols-[1fr_7rem] gap-4">
          <TextField control={form.control} name="city" label="Město" autoComplete="address-level2" />
          <TextField control={form.control} name="zip" label="PSČ" inputMode="numeric" autoComplete="postal-code" />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
