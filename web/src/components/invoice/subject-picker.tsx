// Výběr odběratele ve formuláři faktury: hledání přes GET /subjects?query=
// + „Nový kontakt“ s dohledáním v ARES.

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Building2Icon, ChevronsUpDownIcon, PlusIcon, SearchIcon, UserRoundIcon } from 'lucide-react'
import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import { api, unwrap } from '@/api/client'
import { keys } from '@/api/queries/keys'
import type { ResponseBody } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { NewSubjectDialog } from './new-subject-dialog'
import { useDebouncedValue } from './use-debounced'

export type PickerSubject = ResponseBody<'/api/accounts/{slug}/subjects', 'get'>['items'][number]

/** Co picker zobrazuje jako vybranou hodnotu (u existující faktury snapshot `client_*`). */
export interface SubjectValue {
  id: number
  name: string
  registration_no?: string
  city?: string
}

/** Pod prefixem seznamů kontaktů → po založení kontaktu se obnoví i výběr. */
const pickerKey = (slug: string, query: string) => keys.subjectSearch(slug, query, 30)

function subjectLine(s: { registration_no?: string; city?: string }) {
  return [s.registration_no && `IČO ${s.registration_no}`, s.city].filter(Boolean).join(' · ')
}

interface SubjectPickerProps {
  slug: string
  value: SubjectValue | null
  onChange: (subject: PickerSubject) => void
  invalid?: boolean
  id?: string
  disabled?: boolean
}

export function SubjectPicker({ slug, value, onChange, invalid, id, disabled }: SubjectPickerProps) {
  const [open, setOpen] = useState(false)
  const [creating, setCreating] = useState(false)

  return (
    <>
      <button
        id={id}
        type="button"
        disabled={disabled}
        aria-invalid={invalid || undefined}
        aria-haspopup="dialog"
        onClick={() => setOpen(true)}
        className={cn(
          'flex min-h-14 w-full items-center gap-3 rounded-lg border border-input bg-transparent px-3 py-2 text-left transition-colors outline-none',
          'hover:bg-muted/50 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30 dark:hover:bg-input/50',
          'aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 disabled:opacity-50',
        )}
      >
        <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
          {value ? <Building2Icon className="size-4" /> : <UserRoundIcon className="size-4" />}
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          {value ? (
            <>
              <span className="truncate font-medium">{value.name}</span>
              {subjectLine(value) && <span className="truncate text-xs text-muted-foreground">{subjectLine(value)}</span>}
            </>
          ) : (
            <span className="text-muted-foreground">Vyberte odběratele…</span>
          )}
        </span>
        <ChevronsUpDownIcon className="size-4 shrink-0 text-muted-foreground" />
      </button>

      <ResponsiveDialog
        open={open}
        onOpenChange={setOpen}
        title="Odběratel"
        className="sm:max-w-lg"
      >
        {open && (
          <SubjectSearch
            slug={slug}
            selectedId={value?.id}
            onPick={(s) => {
              onChange(s)
              setOpen(false)
            }}
            onCreate={() => {
              setOpen(false)
              setCreating(true)
            }}
          />
        )}
      </ResponsiveDialog>

      <NewSubjectDialog
        slug={slug}
        open={creating}
        onOpenChange={setCreating}
        onCreated={(s) => {
          onChange(s)
          setCreating(false)
        }}
      />
    </>
  )
}

function SubjectSearch({
  slug,
  selectedId,
  onPick,
  onCreate,
}: {
  slug: string
  selectedId?: number
  onPick: (s: PickerSubject) => void
  onCreate: () => void
}) {
  const [query, setQuery] = useState('')
  const debounced = useDebouncedValue(query.trim(), 250)
  const [active, setActive] = useState(0)
  const listId = useId()
  const listRef = useRef<HTMLUListElement>(null)

  const result = useQuery({
    queryKey: pickerKey(slug, debounced),
    queryFn: ({ signal }) =>
      unwrap(
        api.GET('/api/accounts/{slug}/subjects', {
          params: { path: { slug }, query: { query: debounced || undefined, per_page: 30 } },
          signal,
        }),
      ),
    placeholderData: keepPreviousData,
  })
  const items = result.data?.items ?? []

  useEffect(() => {
    listRef.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: 'nearest' })
  }, [active])

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, items.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const s = items[active]
      if (s) onPick(s)
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <InputGroup>
        <InputGroupAddon>
          <SearchIcon />
        </InputGroupAddon>
        <InputGroupInput
          autoFocus
          type="search"
          inputMode="search"
          enterKeyHint="search"
          placeholder="Název, IČO nebo e-mail"
          aria-label="Hledat kontakt"
          role="combobox"
          aria-expanded="true"
          aria-controls={listId}
          aria-activedescendant={items[active] ? `${listId}-${items[active].id}` : undefined}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            setActive(0)
          }}
          onKeyDown={onKeyDown}
        />
      </InputGroup>

      <Button type="button" variant="outline" className="justify-start" onClick={onCreate}>
        <PlusIcon data-icon="inline-start" />
        Nový kontakt
        <span className="ml-auto text-xs font-normal text-muted-foreground">i z ARES podle IČO</span>
      </Button>

      <ul
        id={listId}
        ref={listRef}
        role="listbox"
        aria-label="Kontakty"
        className="-mx-1 flex max-h-[min(24rem,50dvh)] flex-col gap-0.5 overflow-y-auto px-1"
      >
        {result.isPending &&
          Array.from({ length: 4 }, (_, i) => (
            <li key={i} className="flex flex-col gap-1.5 px-3 py-2.5">
              <Skeleton className="h-4 w-40" />
              <Skeleton className="h-3 w-24" />
            </li>
          ))}
        {!result.isPending && items.length === 0 && (
          <li className="px-3 py-6 text-center text-sm text-muted-foreground">
            {debounced ? `Nic pro „${debounced}“.` : 'Zatím nemáte žádné kontakty.'}
          </li>
        )}
        {items.map((s, i) => (
          <li
            key={s.id}
            id={`${listId}-${s.id}`}
            role="option"
            aria-selected={s.id === selectedId}
            data-index={i}
            data-active={i === active || undefined}
            onMouseMove={() => setActive(i)}
            onClick={() => onPick(s)}
            className={cn(
              'flex min-h-12 cursor-pointer flex-col justify-center rounded-lg px-3 py-2 data-active:bg-muted',
              s.id === selectedId && 'ring-1 ring-primary/40',
            )}
          >
            <span className="truncate text-sm font-medium">{s.name}</span>
            {subjectLine(s) && <span className="truncate text-xs text-muted-foreground">{subjectLine(s)}</span>}
          </li>
        ))}
      </ul>
    </div>
  )
}
