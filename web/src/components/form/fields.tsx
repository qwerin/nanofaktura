// Propojení React Hook Form ↔ shadcn Field. Každé pole: label, popis, chyba, aria-invalid.
//
//   <TextField control={form.control} name="email" label="E-mail" type="email" autoComplete="email" />
//
// Mobile-first: správný `type` / `inputMode` / `autoComplete` je povinný (číselná klávesnice
// pro IČO, PSČ, částky; `email`/`tel` typy; `enterKeyHint`).

import type { ComponentProps, ReactNode } from 'react'
import { useId } from 'react'
import { Controller, type Control, type FieldPath, type FieldValues } from 'react-hook-form'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

interface BaseFieldProps<T extends FieldValues> {
  control: Control<T>
  name: FieldPath<T>
  label: ReactNode
  description?: ReactNode
  className?: string
  disabled?: boolean
}

type InputPassthrough = Pick<
  ComponentProps<'input'>,
  | 'type'
  | 'inputMode'
  | 'autoComplete'
  | 'placeholder'
  | 'maxLength'
  | 'enterKeyHint'
  | 'autoFocus'
  | 'autoCapitalize'
  | 'spellCheck'
  | 'min'
  | 'max'
  | 'step'
  | 'pattern'
>

export function TextField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  disabled,
  ...inputProps
}: BaseFieldProps<T> & InputPassthrough) {
  const id = useId()
  return (
    <Controller
      control={control}
      name={name}
      disabled={disabled}
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined} className={className}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <Input
            id={id}
            {...inputProps}
            {...field}
            value={field.value ?? ''}
            aria-invalid={fieldState.invalid || undefined}
          />
          {description && <FieldDescription>{description}</FieldDescription>}
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}

export function TextareaField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  disabled,
  rows = 3,
  placeholder,
}: BaseFieldProps<T> & { rows?: number; placeholder?: string }) {
  const id = useId()
  return (
    <Controller
      control={control}
      name={name}
      disabled={disabled}
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined} className={className}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <Textarea
            id={id}
            rows={rows}
            placeholder={placeholder}
            {...field}
            value={field.value ?? ''}
            aria-invalid={fieldState.invalid || undefined}
          />
          {description && <FieldDescription>{description}</FieldDescription>}
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}

export interface SelectOption<V extends string = string> {
  value: V
  label: string
}

export function SelectField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  disabled,
  options,
  placeholder = 'Vyberte…',
}: BaseFieldProps<T> & { options: readonly SelectOption[]; placeholder?: string }) {
  const id = useId()
  return (
    <Controller
      control={control}
      name={name}
      disabled={disabled}
      render={({ field, fieldState }) => (
        <Field data-invalid={fieldState.invalid || undefined} className={className}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <Select
            items={options}
            value={field.value ?? null}
            onValueChange={(v) => field.onChange(v)}
            disabled={field.disabled}
          >
            <SelectTrigger
              id={id}
              ref={field.ref}
              onBlur={field.onBlur}
              className="w-full"
              aria-invalid={fieldState.invalid || undefined}
            >
              <SelectValue placeholder={placeholder} />
            </SelectTrigger>
            <SelectContent>
              {options.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {description && <FieldDescription>{description}</FieldDescription>}
          <FieldError errors={[fieldState.error]} />
        </Field>
      )}
    />
  )
}

export function SwitchField<T extends FieldValues>({
  control,
  name,
  label,
  description,
  className,
  disabled,
}: BaseFieldProps<T>) {
  const id = useId()
  return (
    <Controller
      control={control}
      name={name}
      disabled={disabled}
      render={({ field, fieldState }) => (
        <Field
          orientation="horizontal"
          data-invalid={fieldState.invalid || undefined}
          className={cn('rounded-lg border p-3 md:p-4', className)}
        >
          <FieldContent>
            <FieldLabel htmlFor={id}>{label}</FieldLabel>
            {description && <FieldDescription>{description}</FieldDescription>}
          </FieldContent>
          <Switch
            id={id}
            checked={Boolean(field.value)}
            onCheckedChange={(v) => field.onChange(v)}
            onBlur={field.onBlur}
            ref={field.ref}
            disabled={field.disabled}
          />
        </Field>
      )}
    />
  )
}
