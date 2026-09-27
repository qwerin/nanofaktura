import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { Controller, type Control, type FieldPath, type FieldValues } from 'react-hook-form'
import { expenseQueries } from '@/api/queries/expenses'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { AutocompleteInput } from './autocomplete-input'
import { useDebouncedValue } from './use-debounced-value'

interface CategoryFieldProps<T extends FieldValues> {
  control: Control<T>
  name: FieldPath<T>
  label?: string
  description?: string
  disabled?: boolean
}

/** Kategorie nákladu: volný text s našeptáváním už použitých kategorií. */
export function CategoryField<T extends FieldValues>({
  control,
  name,
  label = 'Kategorie',
  description,
  disabled,
}: CategoryFieldProps<T>) {
  const { slug } = useCurrentAccount()
  const id = useId()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(false)
  const debounced = useDebouncedValue(query.trim(), 200)
  const categories = useQuery({ ...expenseQueries.categories(slug, debounced), enabled: active })

  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <AutocompleteInput<string>
            id={id}
            inputRef={field.ref}
            value={String(field.value ?? '')}
            onValueChange={(v) => {
              setActive(true)
              setQuery(v)
              field.onChange(v)
            }}
            onSelect={(c) => {
              setQuery(c)
              field.onChange(c)
            }}
            onBlur={field.onBlur}
            onFocus={() => setActive(true)}
            items={(categories.data ?? []).filter((c) => c !== field.value)}
            itemToString={(c) => c}
            itemKey={(c) => c}
            placeholder="např. Software, Kancelář, Cestovné"
            invalid={fieldState.invalid}
            disabled={disabled}
            inputProps={{ autoComplete: 'off', enterKeyHint: 'next', autoCapitalize: 'sentences' }}
          />
          {description && <FieldDescription>{description}</FieldDescription>}
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}
