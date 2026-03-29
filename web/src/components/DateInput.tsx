import { useState } from 'react'
import { cs } from 'react-day-picker/locale'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Calendar } from '@/components/ui/calendar'
import { cn } from '@/lib/utils'
import { CalendarIcon } from 'lucide-react'

function toDisplay(iso: string): string {
  if (!iso || iso.length !== 10) return ''
  const [y, m, d] = iso.split('-')
  return `${d}.${m}.${y}`
}

function isoToDate(iso: string): Date | undefined {
  if (!iso || iso.length !== 10) return undefined
  const d = new Date(iso + 'T00:00:00')
  return isNaN(d.getTime()) ? undefined : d
}

interface Props {
  value: string          // ISO YYYY-MM-DD
  onChange: (iso: string) => void
  className?: string
}

export function DateInput({ value, onChange, className }: Props) {
  const [open, setOpen] = useState(false)
  const selected = isoToDate(value)

  const handleSelect = (date: Date | undefined) => {
    if (!date) return
    // toLocaleDateString('sv-SE') returns YYYY-MM-DD
    onChange(date.toLocaleDateString('sv-SE'))
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className={cn(
            "flex h-9 items-center gap-2 rounded-md border border-input bg-background px-3 text-sm text-left",
            "hover:bg-accent hover:text-accent-foreground",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            "transition-colors",
            !value && "text-muted-foreground",
            className
          )}
        >
          <CalendarIcon className="h-4 w-4 text-muted-foreground shrink-0" />
          <span>{value ? toDisplay(value) : 'DD.MM.YYYY'}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="single"
          locale={cs}
          selected={selected}
          onSelect={handleSelect}
          defaultMonth={selected ?? new Date()}
          initialFocus
        />
      </PopoverContent>
    </Popover>
  )
}
