import { XIcon } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'
import { cn } from '@/lib/utils'

interface TagInputProps {
  id?: string
  value: string[]
  onChange: (tags: string[]) => void
  placeholder?: string
  disabled?: boolean
}

/** Štítky: Enter nebo čárka přidá, Backspace v prázdném poli odebere poslední. */
export function TagInput({ id, value, onChange, placeholder = 'Přidat štítek…', disabled }: TagInputProps) {
  const [draft, setDraft] = useState('')

  const commit = () => {
    const parts = draft
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
    if (parts.length) onChange([...new Set([...value, ...parts])])
    setDraft('')
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
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
        disabled && 'opacity-50',
      )}
    >
      {value.map((t) => (
        <span key={t} className="inline-flex h-7 items-center gap-1 rounded-md bg-secondary pr-1 pl-2 text-sm md:h-6">
          {t}
          <button
            type="button"
            disabled={disabled}
            aria-label={`Odebrat štítek ${t}`}
            onClick={() => onChange(value.filter((x) => x !== t))}
            className="flex size-5 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <XIcon className="size-3.5" />
          </button>
        </span>
      ))}
      <input
        id={id}
        value={draft}
        disabled={disabled}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKeyDown}
        onBlur={commit}
        placeholder={value.length ? '' : placeholder}
        enterKeyHint="enter"
        autoComplete="off"
        className="h-8 min-w-24 flex-1 bg-transparent text-base outline-none placeholder:text-muted-foreground md:h-6 md:text-sm"
      />
    </div>
  )
}
