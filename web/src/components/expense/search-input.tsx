import { SearchIcon, XIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group'
import { cn } from '@/lib/utils'
import { useDebouncedValue } from './use-debounced-value'

interface SearchInputProps {
  /** Hodnota z URL. */
  value: string
  /** Voláno se zpožděním po psaní (a hned při smazání). */
  onChange: (value: string) => void
  placeholder?: string
  className?: string
  'aria-label'?: string
}

/** Vyhledávací pole seznamu s debounce — hodnota žije v URL, pole ji jen zrcadlí. */
export function SearchInput({ value, onChange, placeholder = 'Hledat…', className, ...rest }: SearchInputProps) {
  const [text, setText] = useState(value)
  const debounced = useDebouncedValue(text, 300)
  const lastSent = useRef(value)

  // Změna URL zvenku (zpět v historii, „Zrušit filtry“) → přepsat pole.
  useEffect(() => {
    if (value !== lastSent.current) {
      lastSent.current = value
      setText(value)
    }
  }, [value])

  useEffect(() => {
    if (debounced !== lastSent.current) {
      lastSent.current = debounced
      onChange(debounced)
    }
  }, [debounced, onChange])

  return (
    <InputGroup className={cn('bg-background', className)}>
      <InputGroupAddon>
        <SearchIcon />
      </InputGroupAddon>
      <InputGroupInput
        type="search"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={placeholder}
        enterKeyHint="search"
        aria-label={rest['aria-label'] ?? placeholder}
        className="[&::-webkit-search-cancel-button]:hidden"
      />
      {text && (
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            size="icon-xs"
            aria-label="Vymazat hledání"
            onClick={() => {
              setText('')
              lastSent.current = ''
              onChange('')
            }}
          >
            <XIcon />
          </InputGroupButton>
        </InputGroupAddon>
      )}
    </InputGroup>
  )
}
