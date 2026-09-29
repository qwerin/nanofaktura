// Export seznamu (CSV / Excel / ZIP s PDF) se stejnými filtry jako seznam (§7.11).
//
// Dvě podoby:
//   - `useExportMenu(...)` (use-export-menu.tsx) → `{ action, drawer }`: položka do `PageHeader.actions`
//     (desktop: tlačítko s rozbalovacím menu, mobil: řádek v menu „…“, který otevře Drawer).
//   - `<ExportMenu …/>`: samostatné tlačítko (desktop dropdown / mobil Drawer).
//
// Soubory se stahují přes fetch → blob (session cookie), s toastem průběhu a čitelnou chybou.

import { DownloadIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from '@/components/ui/drawer'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  exportGroups,
  hasFilters,
  useExportDownload,
  type ExportDownload,
  type ExportGroup,
  type ExportMenuOptions,
} from './export-options'

/** Samostatné tlačítko exportu (mimo PageHeader). */
export function ExportMenu({ className, ...opts }: ExportMenuOptions & { className?: string }) {
  const isMobile = useIsMobile()
  const [open, setOpen] = useState(false)
  const download = useExportDownload()
  const groups = exportGroups(opts.slug, opts.kind, opts.filters)
  const filtered = hasFilters(opts.filters)
  const label = opts.label ?? 'Exportovat'

  if (isMobile) {
    return (
      <>
        <Button variant="outline" size="icon" aria-label={label} className={className} onClick={() => setOpen(true)}>
          {download.busy ? <Spinner /> : <DownloadIcon />}
        </Button>
        <ExportDrawer open={open} onOpenChange={setOpen} groups={groups} download={download} filtered={filtered} />
      </>
    )
  }
  return (
    <ExportDesktopTrigger groups={groups} download={download} filtered={filtered} className={className}>
      <DownloadIcon data-icon="inline-start" />
      {label}
    </ExportDesktopTrigger>
  )
}

/**
 * Spouštěč dropdownu. Použitelný i jako `render` tlačítka v PageHeader — převezme jeho
 * třídy a obsah (ikona + popisek) a vykreslí vlastní tlačítko menu.
 */
export function ExportDesktopTrigger({
  groups,
  download,
  filtered,
  children,
  className,
}: {
  groups: ExportGroup[]
  download: ExportDownload
  filtered: boolean
  children?: ReactNode
  className?: string
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={className} />}
        disabled={Boolean(download.busy)}
      >
        {download.busy ? (
          <>
            <Spinner data-icon="inline-start" />
            Stahuji…
          </>
        ) : (
          children
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        {groups.map((g, gi) => (
          <DropdownMenuGroup key={g.label}>
            {gi > 0 && <DropdownMenuSeparator />}
            <DropdownMenuLabel>{g.label}</DropdownMenuLabel>
            {g.options.map((o) => (
              <DropdownMenuItem key={o.id} onClick={() => void download.run(o)} className="items-start py-1.5">
                <o.icon className="mt-0.5" />
                <span className="flex flex-col">
                  <span>{o.label}</span>
                  <span className="text-xs text-muted-foreground">{o.description}</span>
                </span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        ))}
        <DropdownMenuSeparator />
        <p className="max-w-64 px-2 py-1.5 text-xs text-muted-foreground">
          {filtered ? 'Exportují se doklady podle aktuálních filtrů.' : 'Exportuje se celý seznam (bez filtrů).'}
        </p>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function ExportDrawer({
  open,
  onOpenChange,
  groups,
  download,
  filtered,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  groups: ExportGroup[]
  download: ExportDownload
  filtered: boolean
}) {
  return (
    <Drawer open={open} onOpenChange={onOpenChange} showSwipeHandle>
      <DrawerContent>
        <DrawerHeader className="group-data-[swipe-axis=y]/drawer-popup:text-left">
          <DrawerTitle className="text-lg">Exportovat</DrawerTitle>
          <DrawerDescription>
            {filtered ? 'Podle aktuálních filtrů seznamu.' : 'Celý seznam (bez filtrů).'}
          </DrawerDescription>
        </DrawerHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-2 pt-2 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {groups.map((g) => (
            <section key={g.label} className="flex flex-col gap-1">
              <h3 className="px-3 pb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">{g.label}</h3>
              {g.options.map((o) => (
                <button
                  key={o.id}
                  type="button"
                  disabled={Boolean(download.busy)}
                  onClick={() => {
                    void download.run(o).then(() => onOpenChange(false))
                  }}
                  className="flex min-h-14 w-full items-center gap-3 rounded-lg px-3 py-2 text-left active:bg-muted disabled:opacity-60"
                >
                  {download.busy === o.id ? (
                    <Spinner className="size-5 text-muted-foreground" />
                  ) : (
                    <o.icon className="size-5 shrink-0 text-muted-foreground" />
                  )}
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="text-base">{o.label}</span>
                    <span className="text-sm text-muted-foreground">{o.description}</span>
                  </span>
                </button>
              ))}
            </section>
          ))}
        </div>
      </DrawerContent>
    </Drawer>
  )
}
