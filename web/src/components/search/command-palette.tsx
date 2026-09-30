// Command palette (SPEC §7.15): globální hledání (GET /search) + rychlé akce a navigace.
// Desktop: dialog nahoře (⌘K / Ctrl+K, „/“). Mobil: celá obrazovka (tlačítko lupy v hlavičce).

import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Command as CommandPrimitive } from 'cmdk'
import {
  ArrowRightIcon,
  CornerDownLeftIcon,
  FilePlusIcon,
  FileTextIcon,
  KeyboardIcon,
  PackageIcon,
  ReceiptIcon,
  SearchIcon,
  UserPlusIcon,
  UserIcon,
  XIcon,
  type LucideIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { searchQueries } from '@/api/queries/search'
import type { ExpenseStatus, InvoiceStatus, SearchHit } from '@/api/types'
import { ExpenseStatusBadge } from '@/components/expense/expense-status-badge'
import { useDebouncedValue } from '@/components/invoice/use-debounced'
import { StatusBadge } from '@/components/invoice/status-badge'
import { adminNav, mainNav, settingsNav } from '@/components/layout/nav'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CommandEmpty, CommandGroup, CommandList, CommandSeparator } from '@/components/ui/command'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { useIsMobile } from '@/hooks/use-mobile'
import { cn } from '@/lib/utils'
import { countResults, matchesQuery, resultGroups } from './palette-actions'

const invoiceStatuses = new Set<string>(['open', 'sent', 'overdue', 'paid', 'cancelled', 'uncollectible'])
const expenseStatuses = new Set<string>(['open', 'overdue', 'paid'])

const hitIcons: Record<SearchHit['type'], LucideIcon> = {
  invoice: FileTextIcon,
  expense: ReceiptIcon,
  subject: UserIcon,
  price_item: PackageIcon,
}

interface PaletteAction {
  id: string
  label: string
  icon: LucideIcon
  keywords?: string[]
  shortcut?: string
  run: () => void
}

export function CommandPalette({
  open,
  onOpenChange,
  initialQuery = '',
  onShowHelp,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialQuery?: string
  onShowHelp: () => void
}) {
  const isMobile = useIsMobile()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className={cn(
          'flex flex-col gap-0 overflow-hidden p-0',
          // mobil: celá obrazovka
          'max-md:inset-0 max-md:h-dvh max-md:max-w-none max-md:translate-x-0 max-md:translate-y-0 max-md:rounded-none max-md:ring-0',
          // desktop: nahoře, širší
          'md:top-[12vh] md:max-h-[min(34rem,76vh)] md:translate-y-0 sm:max-w-xl',
        )}
      >
        <DialogTitle className="sr-only">Hledat</DialogTitle>
        <DialogDescription className="sr-only">Hledejte faktury, náklady, kontakty a položky ceníku nebo spusťte akci.</DialogDescription>
        {/* Obsah se připojí znovu při každém otevření → čistý stav (dotaz, výběr). */}
        {open && (
          <PaletteBody
            initialQuery={initialQuery}
            isMobile={isMobile}
            close={() => onOpenChange(false)}
            onShowHelp={onShowHelp}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function PaletteBody({
  initialQuery,
  isMobile,
  close,
  onShowHelp,
}: {
  initialQuery: string
  isMobile: boolean
  close: () => void
  onShowHelp: () => void
}) {
  const { slug, canEdit, me } = useCurrentAccount()
  const navigate = useNavigate()
  const [query, setQuery] = useState(initialQuery)
  const [selected, setSelected] = useState('')
  const debounced = useDebouncedValue(query.trim(), 200)
  const search = useQuery(searchQueries.global(slug, debounced))
  const hasQuery = query.trim().length > 0
  // Výsledky ke starému dotazu po smazání pole neukazovat.
  const results = hasQuery && debounced ? search.data : undefined
  const loading = hasQuery && (debounced !== query.trim() || search.isFetching)

  const go = (href: string) => {
    close()
    void navigate({ href })
  }

  const actions: PaletteAction[] = []
  if (canEdit) {
    actions.push(
      {
        id: 'new-invoice',
        label: 'Nová faktura',
        icon: FilePlusIcon,
        keywords: ['vystavit', 'doklad', 'fakturovat'],
        shortcut: 'N',
        run: () => go(`/a/${slug}/invoices/new`),
      },
      {
        id: 'new-expense',
        label: 'Nový náklad',
        icon: ReceiptIcon,
        keywords: ['výdaj', 'přijatá faktura', 'účtenka'],
        run: () => go(`/a/${slug}/expenses/new`),
      },
      {
        id: 'new-subject',
        label: 'Nový kontakt',
        icon: UserPlusIcon,
        keywords: ['klient', 'odběratel', 'dodavatel', 'firma'],
        run: () => go(`/a/${slug}/subjects/new`),
      },
    )
  }
  if (me?.instance_admin) {
    actions.push({
      id: 'admin',
      label: adminNav.label,
      icon: adminNav.icon,
      keywords: ['server', 'instance', 'administrace', 'uživatelé'],
      run: () => go(`/a/${slug}/admin`),
    })
  }
  if (me?.instance_admin && hasQuery) {
    actions.push(
      {
        id: 'admin-email-test',
        label: 'Test e-mailu',
        icon: adminNav.icon,
        keywords: ['smtp', 'dkim', 'spf', 'dmarc', 'pošta', 'správa instance'],
        run: () => go(`/a/${slug}/admin?tab=email`),
      },
    )
  }
  if (!isMobile) {
    actions.push({
      id: 'help',
      label: 'Klávesové zkratky',
      icon: KeyboardIcon,
      keywords: ['zkratky', 'nápověda'],
      shortcut: '?',
      run: () => {
        close()
        onShowHelp()
      },
    })
  }

  const navItems = [
    ...mainNav.map((i) => ({ id: `nav:${i.to}`, label: i.label, icon: i.icon, href: i.to.replace('$slug', slug) })),
    ...settingsNav.map((i) => ({ id: `nav:${i.to}`, label: `Nastavení › ${i.label}`, icon: i.icon, href: i.to.replace('$slug', slug) })),
  ]

  const shownActions = actions.filter((a) => matchesQuery(query, a.label, a.keywords))
  // Bez dotazu jen hlavní sekce, s dotazem i nastavení.
  const shownNav = hasQuery ? navItems.filter((n) => matchesQuery(query, n.label)).slice(0, 6) : navItems.filter((n) => !n.label.startsWith('Nastavení'))
  const total = countResults(results)
  // „Nová faktura pro …“ se ukáže pod kontaktem, na kterém je výběr (a zůstane, když na ni výběr přejde).
  const selectedSubjectId = /^(?:subject|new-invoice-for):(\d+)$/.exec(selected)?.[1]

  return (
    <CommandPrimitive
      shouldFilter={false}
      loop
      value={selected}
      onValueChange={setSelected}
      label="Hledat"
      className="flex min-h-0 flex-1 flex-col bg-popover text-popover-foreground"
    >
      {/* Pole hledání */}
      <div className="flex items-center gap-2 border-b px-3 pt-safe md:px-4">
        <SearchIcon className="size-5 shrink-0 text-muted-foreground md:size-4" aria-hidden="true" />
        <CommandPrimitive.Input
          value={query}
          onValueChange={setQuery}
          autoFocus
          placeholder="Hledat faktury, kontakty, náklady…"
          enterKeyHint="go"
          autoComplete="off"
          autoCorrect="off"
          spellCheck={false}
          className="h-14 min-w-0 flex-1 bg-transparent text-base outline-hidden placeholder:text-muted-foreground md:h-12 md:text-sm"
        />
        {loading && <Spinner className="text-muted-foreground" />}
        {isMobile ? (
          <Button variant="ghost" className="-mr-2 shrink-0 text-primary" onClick={close}>
            Zrušit
          </Button>
        ) : (
          query && (
            <Button variant="ghost" size="icon-sm" aria-label="Vymazat" onClick={() => setQuery('')}>
              <XIcon />
            </Button>
          )
        )}
      </div>

      <CommandList className="max-h-none min-h-0 flex-1 overscroll-contain px-1 py-1 pb-safe md:max-h-[min(28rem,60vh)]">
        {hasQuery && !loading && total === 0 && shownActions.length === 0 && shownNav.length === 0 && (
          <CommandEmpty className="py-10 text-muted-foreground">Nic nenalezeno pro „{query.trim()}“.</CommandEmpty>
        )}
        {search.isError && hasQuery && (
          <p className="px-3 py-3 text-sm text-destructive">Hledání se nezdařilo. Zkuste to znovu.</p>
        )}

        {results &&
          resultGroups.map((g) => {
            const hits = results[g.key] ?? []
            if (hits.length === 0) return null
            return (
              <CommandGroup key={g.key} heading={g.heading}>
                {hits.map((hit) => (
                  <HitItems
                    key={`${hit.type}:${hit.id}`}
                    hit={hit}
                    onOpen={() => go(hit.url_hint)}
                    extra={
                      canEdit && hit.type === 'subject' && selectedSubjectId === String(hit.id) ? (
                        <PaletteItem
                          value={`new-invoice-for:${hit.id}`}
                          icon={FilePlusIcon}
                          className="ml-6"
                          onSelect={() => go(`/a/${slug}/invoices/new?subject_id=${hit.id}`)}
                        >
                          Nová faktura pro <strong className="font-semibold">{hit.title}</strong>
                        </PaletteItem>
                      ) : null
                    }
                  />
                ))}
              </CommandGroup>
            )
          })}

        {shownActions.length > 0 && (
          <>
            {results && total > 0 && <CommandSeparator className="my-1" />}
            <CommandGroup heading="Akce">
              {shownActions.map((a) => (
                <PaletteItem key={a.id} value={`action:${a.id}`} icon={a.icon} shortcut={isMobile ? undefined : a.shortcut} onSelect={a.run}>
                  {a.label}
                </PaletteItem>
              ))}
            </CommandGroup>
          </>
        )}

        {shownNav.length > 0 && (
          <CommandGroup heading="Přejít na">
            {shownNav.map((n) => (
              <PaletteItem key={n.id} value={n.id} icon={n.icon} onSelect={() => go(n.href)} trailing={<ArrowRightIcon className="text-muted-foreground" />}>
                {n.label}
              </PaletteItem>
            ))}
          </CommandGroup>
        )}
      </CommandList>

      {!isMobile && (
        <div className="flex items-center gap-4 border-t px-4 py-2 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd> vybrat
          </span>
          <span className="flex items-center gap-1">
            <Kbd>
              <CornerDownLeftIcon className="size-3" />
            </Kbd>{' '}
            otevřít
          </span>
          <span className="flex items-center gap-1">
            <Kbd>esc</Kbd> zavřít
          </span>
        </div>
      )}
    </CommandPrimitive>
  )
}

function HitItems({ hit, onOpen, extra }: { hit: SearchHit; onOpen: () => void; extra: ReactNode }) {
  const Icon = hitIcons[hit.type]
  return (
    <>
      <PaletteItem value={`${hit.type}:${hit.id}`} icon={Icon} onSelect={onOpen} trailing={<HitStatus hit={hit} />}>
        <span className="flex min-w-0 flex-col">
          <span className="truncate font-medium">{hit.title}</span>
          {hit.subtitle && <span className="truncate text-xs text-muted-foreground">{hit.subtitle}</span>}
        </span>
      </PaletteItem>
      {extra}
    </>
  )
}

function HitStatus({ hit }: { hit: SearchHit }) {
  if (!hit.status) return null
  if (hit.type === 'invoice' && invoiceStatuses.has(hit.status)) return <StatusBadge status={hit.status as InvoiceStatus} />
  if (hit.type === 'expense' && expenseStatuses.has(hit.status)) return <ExpenseStatusBadge status={hit.status as ExpenseStatus} />
  if (hit.type === 'price_item' && hit.status === 'archived') return <Badge variant="outline">Archivováno</Badge>
  return null
}

function PaletteItem({
  value,
  icon: Icon,
  children,
  onSelect,
  shortcut,
  trailing,
  className,
}: {
  value: string
  icon: LucideIcon
  children: ReactNode
  onSelect: () => void
  shortcut?: string
  trailing?: ReactNode
  className?: string
}) {
  return (
    <CommandPrimitive.Item
      value={value}
      onSelect={onSelect}
      className={cn(
        'relative flex min-h-12 cursor-default items-center gap-3 rounded-lg px-3 py-1.5 text-sm outline-hidden select-none data-[selected=true]:bg-muted data-[selected=true]:text-foreground md:min-h-10',
        className,
      )}
    >
      <Icon className="size-4 text-muted-foreground" aria-hidden="true" />
      <span className="flex min-w-0 flex-1 items-center gap-1">{children}</span>
      {trailing && <span className="flex shrink-0 items-center">{trailing}</span>}
      {shortcut && <Kbd className="shrink-0">{shortcut}</Kbd>}
    </CommandPrimitive.Item>
  )
}

export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        'inline-flex h-5 min-w-5 items-center justify-center rounded border bg-muted px-1 font-sans text-[11px] font-medium text-muted-foreground',
        className,
      )}
    >
      {children}
    </kbd>
  )
}
