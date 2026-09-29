import { PlusIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { MAX_REMINDER_DAY, normalizeReminderDays, REMINDER_PRESETS } from './reminders'

/** Editor kroků upomínek: čipy s dny po splatnosti, přidání vlastního počtu dní a rychlé předvolby. */
export function ReminderDaysInput({
  id,
  value,
  onChange,
  disabled,
}: {
  id?: string
  value: number[]
  onChange: (days: number[]) => void
  disabled?: boolean
}) {
  const [draft, setDraft] = useState('')
  const n = Number(draft)
  const draftValid = /^\d{1,3}$/.test(draft) && n >= 1 && n <= MAX_REMINDER_DAY && !value.includes(n)
  const add = (d: number) => onChange(normalizeReminderDays([...value, d]))

  return (
    <div className="flex flex-col gap-3">
      <ul className="flex flex-wrap gap-2" aria-label="Upomínky">
        {value.length === 0 && <li className="text-sm text-muted-foreground">Žádný krok — upomínky se nebudou posílat.</li>}
        {value.map((d, i) => (
          <li
            key={d}
            className="inline-flex h-9 items-center gap-1 rounded-full border bg-background pr-1 pl-3 text-sm md:h-8 dark:bg-input/30"
          >
            <span className="text-xs text-muted-foreground">{i + 1}.</span>
            <span className="font-medium tabular-nums">+{d}</span>
            <span className="text-muted-foreground">dní</span>
            <button
              type="button"
              disabled={disabled}
              onClick={() => onChange(value.filter((x) => x !== d))}
              aria-label={`Odebrat upomínku ${d} dní po splatnosti`}
              className="ml-0.5 flex size-7 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50 md:size-6"
            >
              <XIcon className="size-3.5" />
            </button>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex items-center gap-2">
          <Input
            id={id}
            value={draft}
            onChange={(e) => setDraft(e.target.value.replace(/\D/g, ''))}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                if (draftValid) {
                  add(n)
                  setDraft('')
                }
              }
            }}
            inputMode="numeric"
            enterKeyHint="done"
            maxLength={3}
            placeholder="dní"
            className="w-20"
            disabled={disabled}
            aria-label="Počet dní po splatnosti"
          />
          <Button
            type="button"
            variant="outline"
            disabled={disabled || !draftValid}
            onClick={() => {
              add(n)
              setDraft('')
            }}
          >
            <PlusIcon data-icon="inline-start" />
            Přidat
          </Button>
        </div>
        <div className="flex flex-wrap gap-1">
          {REMINDER_PRESETS.filter((p) => !value.includes(p)).map((p) => (
            <button
              key={p}
              type="button"
              disabled={disabled}
              onClick={() => add(p)}
              className={cn(
                'h-9 rounded-md px-2.5 text-sm text-muted-foreground tabular-nums hover:bg-muted hover:text-foreground md:h-7',
                'disabled:opacity-50',
              )}
            >
              +{p}
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
