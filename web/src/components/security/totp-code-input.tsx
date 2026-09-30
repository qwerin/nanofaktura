import { useId } from 'react'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export const TOTP_LENGTH = 6

/**
 * Pole pro 6místný kód z ověřovací aplikace: číselná klávesnice, automatické vyplnění z SMS/klíčenky
 * (`one-time-code`), nečíselné znaky se zahodí. Po zadání 6. číslice zavolá `onComplete`.
 */
export function TotpCodeInput({
  value,
  onChange,
  onComplete,
  error,
  label = 'Kód z aplikace',
  description,
  disabled,
  autoFocus,
  className,
}: {
  value: string
  onChange: (value: string) => void
  onComplete?: (code: string) => void
  error?: string
  label?: string
  description?: string
  /** Probíhá ověření — pole nejde měnit, ale zůstává zaostřené. */
  disabled?: boolean
  autoFocus?: boolean
  className?: string
}) {
  const id = useId()
  return (
    <Field data-invalid={Boolean(error) || undefined} className={className}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        name="one-time-code"
        value={value}
        onChange={(e) => {
          const next = e.target.value.replace(/\D/g, '').slice(0, TOTP_LENGTH)
          onChange(next)
          if (next.length === TOTP_LENGTH && next !== value) onComplete?.(next)
        }}
        type="text"
        inputMode="numeric"
        pattern="[0-9]*"
        autoComplete="one-time-code"
        enterKeyHint="go"
        maxLength={TOTP_LENGTH}
        placeholder="123456"
        // readOnly místo disabled: pole si během ověřování drží fokus, po chybě se dá hned psát znovu
        readOnly={disabled}
        aria-busy={disabled || undefined}
        autoFocus={autoFocus}
        aria-invalid={Boolean(error) || undefined}
        className={cn('h-12 text-center font-mono text-2xl tracking-[0.4em] md:h-11 md:text-xl')}
      />
      {description && <FieldDescription>{description}</FieldDescription>}
      <FieldError>{error}</FieldError>
    </Field>
  )
}
