import { XIcon } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'
import { cn } from '@/lib/utils'
import { isEmailLike, splitAddresses } from './placeholders'

interface RecipientsInputProps {
  id?: string
  value: string[]
  onChange: (value: string[]) => void
  placeholder?: string
  disabled?: boolean
  invalid?: boolean
  /** Indexy adres, které odmítl server (422 `body.to[i]`). */
  invalidIndexes?: number[]
}

/**
 * Adresy jako čipy: Enter, čárka, středník nebo mezera adresu potvrdí, Backspace v prázdném poli
 * odebere poslední. Vložení seznamu (paste) se rozdělí. Neplatně vypadající adresy jsou červené.
 */
export function RecipientsInput({ id, value, onChange, placeholder, disabled, invalid, invalidIndexes = [] }: RecipientsInputProps) {
  const [draft, setDraft] = useState('')

  const commit = (text = draft) => {
    const parts = splitAddresses(text).filter((a) => !value.some((v) => v.toLowerCase() === a.toLowerCase()))
    if (parts.length) onChange([...value, ...parts])
    setDraft('')
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === ';' || e.key === ' ') {
      if (!draft.trim()) {
        if (e.key === 'Enter') return
        e.preventDefault()
        return
      }
      e.preventDefault()
      commit()
    } else if (e.key === 'Backspace' && !draft && value.length) {
      onChange(value.slice(0, -1))
    }
  }

  return (
    <div
      className={cn(
        'flex min-h-11 w-full flex-wrap items-center gap-1.5 rounded-lg border border-input px-2 py-1.5 md:min-h-8 md:py-1 dark:bg-input/30',
        'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
        invalid && 'border-destructive ring-3 ring-destructive/20',
        disabled && 'opacity-50',
      )}
    >
      {value.map((addr, i) => {
        const bad = !isEmailLike(addr) || invalidIndexes.includes(i)
        return (
          <span
            key={addr}
            className={cn(
              'inline-flex h-7 max-w-full items-center gap-1 rounded-md bg-secondary pr-1 pl-2 text-sm md:h-6',
              bad && 'bg-destructive/10 text-destructive',
            )}
          >
            <span className="truncate">{addr}</span>
            <button
              type="button"
              disabled={disabled}
              aria-label={`Odebrat ${addr}`}
              onClick={() => onChange(value.filter((x) => x !== addr))}
              className="flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <XIcon className="size-3.5" />
            </button>
          </span>
        )
      })}
      <input
        id={id}
        type="email"
        inputMode="email"
        autoCapitalize="none"
        spellCheck={false}
        value={draft}
        disabled={disabled}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKeyDown}
        onBlur={() => commit()}
        onPaste={(e) => {
          const text = e.clipboardData.getData('text')
          if (/[\s,;]/.test(text.trim())) {
            e.preventDefault()
            commit(draft + text)
          }
        }}
        placeholder={value.length ? '' : placeholder}
        enterKeyHint="enter"
        autoComplete="email"
        aria-invalid={invalid || undefined}
        className="h-8 min-w-32 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground md:h-6 md:text-sm"
      />
    </div>
  )
}
