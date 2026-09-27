import { Autocomplete } from '@base-ui/react/autocomplete'
import type { ComponentProps, ReactNode, Ref } from 'react'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

interface AutocompleteInputProps<T> {
  id?: string
  value: string
  /** Změna textu (psaní). Výběr položky hlásí `onSelect`. */
  onValueChange: (value: string) => void
  onSelect: (item: T) => void
  onBlur?: () => void
  onFocus?: () => void
  /** Položky už filtrované serverem. */
  items: readonly T[]
  itemToString: (item: T) => string
  itemKey: (item: T) => string | number
  renderItem?: (item: T) => ReactNode
  loading?: boolean
  empty?: ReactNode
  placeholder?: string
  inputRef?: Ref<HTMLInputElement>
  invalid?: boolean
  disabled?: boolean
  inputProps?: Pick<ComponentProps<'input'>, 'autoComplete' | 'enterKeyHint' | 'autoCapitalize' | 'name' | 'maxLength'>
  className?: string
}

/**
 * Textové pole s našeptávačem (volný text + návrhy ze serveru). Postavené přímo na Base UI Autocomplete,
 * vzhled pole je shodný s `Input` (na mobilu h-11).
 */
export function AutocompleteInput<T>({
  id,
  value,
  onValueChange,
  onSelect,
  onBlur,
  onFocus,
  items,
  itemToString,
  itemKey,
  renderItem,
  loading,
  empty,
  placeholder,
  inputRef,
  invalid,
  disabled,
  inputProps,
  className,
}: AutocompleteInputProps<T>) {
  return (
    <Autocomplete.Root
      items={items}
      value={value}
      filter={null}
      openOnInputClick
      itemToStringValue={itemToString}
      onValueChange={(v, details) => {
        if (details.reason === 'item-press') {
          const item = items.find((i) => itemToString(i) === v)
          if (item !== undefined) {
            onSelect(item)
            return
          }
        }
        onValueChange(v)
      }}
      disabled={disabled}
    >
      <div className={cn('relative', className)}>
        <Autocomplete.Input
          id={id}
          ref={inputRef}
          placeholder={placeholder}
          onBlur={onBlur}
          onFocus={onFocus}
          aria-invalid={invalid || undefined}
          render={<Input />}
          {...inputProps}
        />
        {loading && <Spinner className="absolute top-1/2 right-3 -translate-y-1/2 text-muted-foreground" />}
      </div>
      <Autocomplete.Portal>
        <Autocomplete.Positioner sideOffset={4} align="start" className="isolate z-50">
          <Autocomplete.Popup
            className={cn(
              'max-h-[min(20rem,var(--available-height))] w-(--anchor-width) max-w-(--available-width) overflow-y-auto overscroll-contain rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10',
              !empty && 'data-[empty]:hidden',
            )}
          >
            {empty && <Autocomplete.Empty className="px-2 py-2 text-sm text-muted-foreground empty:hidden">{empty}</Autocomplete.Empty>}
            <Autocomplete.List>
              {(item: T) => (
                <Autocomplete.Item
                  key={itemKey(item)}
                  value={item}
                  className="flex min-h-11 cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none select-none data-highlighted:bg-accent data-highlighted:text-accent-foreground md:min-h-8"
                >
                  {renderItem ? renderItem(item) : itemToString(item)}
                </Autocomplete.Item>
              )}
            </Autocomplete.List>
          </Autocomplete.Popup>
        </Autocomplete.Positioner>
      </Autocomplete.Portal>
    </Autocomplete.Root>
  )
}
