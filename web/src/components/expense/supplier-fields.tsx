import { useQuery } from '@tanstack/react-query'
import { BookUserIcon, XIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller, useWatch, type UseFormReturn } from 'react-hook-form'
import { expenseQueries, type SupplierOption } from '@/api/queries/expenses'
import { TextField } from '@/components/form/fields'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { AutocompleteInput } from './autocomplete-input'
import type { ExpenseFormValues } from './expense-form-schema'
import { useDebouncedValue } from './use-debounced-value'

/**
 * Dodavatel nákladu: našeptávač z kontaktů (typ dodavatel) s fallbackem na volný text
 * (účtenka bez kontaktu — SPEC: `subject_id` je volitelné, pak je povinné `supplier_name`).
 */
export function SupplierFields({ form, disabled }: { form: UseFormReturn<ExpenseFormValues>; disabled?: boolean }) {
  const { slug } = useCurrentAccount()
  const id = useId()
  const subjectId = useWatch({ control: form.control, name: 'subject_id' })
  const name = useWatch({ control: form.control, name: 'supplier_name' })
  const [typing, setTyping] = useState(false)
  const debounced = useDebouncedValue(typing ? name.trim() : '')
  const suggestions = useQuery({ ...expenseQueries.supplierSearch(slug, debounced), enabled: typing && !subjectId })

  const pick = (s: SupplierOption) => {
    form.setValue('subject_id', s.id, { shouldDirty: true })
    form.setValue('supplier_name', s.name, { shouldDirty: true, shouldValidate: true })
    form.setValue('supplier_registration_no', s.registration_no, { shouldDirty: true })
    form.setValue('supplier_vat_no', s.vat_no, { shouldDirty: true })
    setTyping(false)
  }

  const unlink = () => {
    form.setValue('subject_id', undefined, { shouldDirty: true })
  }

  return (
    <div className="flex flex-col gap-5">
      <Controller
        control={form.control}
        name="supplier_name"
        render={({ field, fieldState }) => (
          <Field data-invalid={fieldState.invalid || undefined}>
            <FieldLabel htmlFor={id}>Dodavatel</FieldLabel>
            {subjectId ? (
              <div className="flex min-h-11 items-center gap-3 rounded-lg border bg-muted/40 px-3 py-2 md:min-h-8 md:py-1">
                <BookUserIcon className="size-4 shrink-0 text-primary" />
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm font-medium">{field.value}</span>
                  {(form.getValues('supplier_registration_no') || form.getValues('supplier_vat_no')) && (
                    <span className="truncate text-xs text-muted-foreground">
                      {[form.getValues('supplier_registration_no') && `IČO ${form.getValues('supplier_registration_no')}`, form.getValues('supplier_vat_no') && `DIČ ${form.getValues('supplier_vat_no')}`]
                        .filter(Boolean)
                        .join(' · ')}
                    </span>
                  )}
                </div>
                <Button type="button" variant="ghost" size="icon-sm" aria-label="Změnit dodavatele" onClick={unlink} disabled={disabled}>
                  <XIcon />
                </Button>
              </div>
            ) : (
              <AutocompleteInput<SupplierOption>
                id={id}
                inputRef={field.ref}
                value={field.value}
                onValueChange={(v) => {
                  setTyping(true)
                  field.onChange(v)
                }}
                onSelect={pick}
                onBlur={field.onBlur}
                items={typing && name.trim() ? (suggestions.data ?? []) : []}
                loading={suggestions.isFetching && typing}
                itemToString={(s) => s.name}
                itemKey={(s) => s.id}
                renderItem={(s) => (
                  <div className="flex min-w-0 flex-col">
                    <span className="truncate">{s.name}</span>
                    {(s.registration_no || s.city) && (
                      <span className="truncate text-xs text-muted-foreground">
                        {[s.registration_no && `IČO ${s.registration_no}`, s.city].filter(Boolean).join(' · ')}
                      </span>
                    )}
                  </div>
                )}
                placeholder="Hledat v kontaktech nebo napsat název"
                invalid={fieldState.invalid}
                disabled={disabled}
                inputProps={{ autoComplete: 'off', enterKeyHint: 'next', autoCapitalize: 'words' }}
              />
            )}
            <FieldDescription>
              {subjectId
                ? 'Údaje dodavatele se převezmou z adresáře kontaktů.'
                : 'Vyberte kontakt, nebo jen napište název (např. u účtenky z obchodu).'}
            </FieldDescription>
            <FieldError errors={[fieldState.error]} />
          </Field>
        )}
      />
      {!subjectId && (
        <div className="grid gap-5 sm:grid-cols-2">
          <TextField control={form.control} name="supplier_registration_no" label="IČO dodavatele" inputMode="numeric" autoComplete="off" enterKeyHint="next" disabled={disabled} maxLength={12} />
          <TextField control={form.control} name="supplier_vat_no" label="DIČ dodavatele" autoComplete="off" autoCapitalize="characters" enterKeyHint="next" disabled={disabled} maxLength={20} />
        </div>
      )}
    </div>
  )
}
