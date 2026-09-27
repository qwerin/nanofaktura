import { CircleCheckIcon, SearchIcon } from 'lucide-react'
import { useId, useRef, useState, type ReactNode } from 'react'
import { Controller, type Control, type FieldPath, type FieldValues } from 'react-hook-form'
import { aresErrorMessage, useAresLookup } from '@/api/queries/ares'
import type { AresSubject } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { isValidIco } from '@/lib/validation'
import { cn } from '@/lib/utils'

interface IcoAresFieldProps<T extends FieldValues> {
  control: Control<T>
  name: FieldPath<T>
  /** Zavolá se s daty z ARES — typicky doplní název, DIČ a adresu. */
  onLoaded: (data: AresSubject) => void
  /** Po napsání platného 8místného IČO se ARES dotáže sám (výchozí `true`). */
  autoLookup?: boolean
  label?: ReactNode
  description?: ReactNode
  className?: string
  autoFocus?: boolean
  size?: 'default' | 'lg'
}

type Status = { kind: 'idle' } | { kind: 'ok'; name: string } | { kind: 'error'; message: string }

/**
 * Pole IČO s tlačítkem „Načíst z ARES“. Kontrolní součet IČO se ověřuje v zod schématu formuláře
 * (`optionalIcoSchema`); tady jen řídíme dotaz na ARES a ukazujeme jeho výsledek.
 */
export function IcoAresField<T extends FieldValues>({
  control,
  name,
  onLoaded,
  autoLookup = true,
  label = 'IČO',
  description,
  className,
  autoFocus,
  size = 'default',
}: IcoAresFieldProps<T>) {
  const id = useId()
  const statusId = useId()
  const lookup = useAresLookup()
  const [status, setStatus] = useState<Status>({ kind: 'idle' })
  const lastLookup = useRef<string | null>(null)

  const run = (raw: string) => {
    const ico = raw.trim().padStart(8, '0')
    if (!isValidIco(raw.trim())) {
      setStatus({ kind: 'error', message: 'Zadejte platné IČO (8 číslic).' })
      return
    }
    lastLookup.current = ico
    setStatus({ kind: 'idle' })
    lookup.mutate(ico, {
      onSuccess: (data) => {
        setStatus({ kind: 'ok', name: data.name })
        onLoaded(data)
      },
      onError: (err) => setStatus({ kind: 'error', message: aresErrorMessage(err) }),
    })
  }

  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => {
        const value = String(field.value ?? '')
        return (
          <Field data-invalid={fieldState.invalid || undefined} className={className}>
            <FieldLabel htmlFor={id}>{label}</FieldLabel>
            <div className="flex gap-2">
              <Input
                id={id}
                ref={field.ref}
                name={field.name}
                value={value}
                onBlur={field.onBlur}
                disabled={field.disabled}
                onChange={(e) => {
                  const v = e.target.value.replace(/\s+/g, '')
                  field.onChange(v)
                  if (status.kind !== 'idle') setStatus({ kind: 'idle' })
                  if (autoLookup && /^\d{8}$/.test(v) && isValidIco(v) && v !== lastLookup.current) run(v)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    run(value)
                  }
                }}
                inputMode="numeric"
                autoComplete="off"
                enterKeyHint="search"
                maxLength={8}
                placeholder="12345678"
                autoFocus={autoFocus}
                aria-invalid={fieldState.invalid || undefined}
                aria-describedby={statusId}
                className={cn('min-w-0 flex-1 font-mono tracking-wide', size === 'lg' && 'h-12 text-lg md:h-10 md:text-base')}
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => run(value)}
                disabled={lookup.isPending || field.disabled || value.trim() === ''}
                className={cn('shrink-0', size === 'lg' && 'h-12 md:h-10')}
              >
                {lookup.isPending ? <Spinner data-icon="inline-start" /> : <SearchIcon data-icon="inline-start" />}
                Načíst z ARES
              </Button>
            </div>
            <div id={statusId} aria-live="polite">
              {lookup.isPending ? (
                <FieldDescription>Hledám v ARES…</FieldDescription>
              ) : status.kind === 'ok' ? (
                <p className="flex items-center gap-1.5 text-sm text-success">
                  <CircleCheckIcon className="size-4 shrink-0" />
                  <span className="min-w-0 truncate">Načteno z ARES: {status.name}</span>
                </p>
              ) : status.kind === 'error' && !fieldState.invalid ? (
                <p className="text-sm text-destructive">{status.message}</p>
              ) : (
                description && !fieldState.invalid && <FieldDescription>{description}</FieldDescription>
              )}
            </div>
            <FieldError errors={[fieldState.error]} />
          </Field>
        )
      }}
    />
  )
}
