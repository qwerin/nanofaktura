import { useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { isMacPlatform, matchShortcut, shortcutList } from '@/lib/shortcuts'
import { CommandPalette, Kbd } from './command-palette'
import { CommandPaletteContext, type CommandPaletteApi } from './palette-context'

/**
 * Paleta + globální klávesové zkratky přihlášené části (⌘K/Ctrl+K, „/“, „N“, „?“).
 * Zkratky mimo ⌘K se ignorují při psaní do pole a při otevřeném dialogu.
 */
export function CommandPaletteProvider({ children }: { children: ReactNode }) {
  const { slug, canEdit } = useCurrentAccount()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [initialQuery, setInitialQuery] = useState('')
  const [helpOpen, setHelpOpen] = useState(false)

  const openPalette = useCallback((query = '') => {
    setInitialQuery(query)
    setHelpOpen(false)
    setOpen(true)
  }, [])
  const api = useMemo<CommandPaletteApi>(() => ({ open: openPalette, openHelp: () => setHelpOpen(true) }), [openPalette])

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return
      const action = matchShortcut(e)
      if (!action) return
      if (action === 'palette') {
        e.preventDefault()
        setOpen((o) => {
          if (!o) setInitialQuery('')
          return !o
        })
        setHelpOpen(false)
        return
      }
      // Jiný otevřený dialog/drawer (formulář, potvrzení …) má přednost.
      if (open || helpOpen || document.querySelector('[role="dialog"], [role="alertdialog"]')) return
      e.preventDefault()
      if (action === 'search') openPalette()
      else if (action === 'help') setHelpOpen(true)
      else if (action === 'new-invoice' && canEdit) void navigate({ to: '/a/$slug/invoices/new', params: { slug } })
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, helpOpen, canEdit, slug, navigate, openPalette])

  return (
    <CommandPaletteContext.Provider value={api}>
      {children}
      <CommandPalette open={open} onOpenChange={setOpen} initialQuery={initialQuery} onShowHelp={() => setHelpOpen(true)} />
      <ShortcutsHelpDialog open={helpOpen} onOpenChange={setHelpOpen} />
    </CommandPaletteContext.Provider>
  )
}

function ShortcutsHelpDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const items = shortcutList(isMacPlatform())
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Klávesové zkratky</DialogTitle>
          <DialogDescription>Fungují kdekoli mimo textová pole.</DialogDescription>
        </DialogHeader>
        <dl className="flex flex-col divide-y">
          {items.map((s) => (
            <div key={s.label} className="flex items-center justify-between gap-4 py-2">
              <dt className="text-sm">{s.label}</dt>
              <dd className="flex shrink-0 items-center gap-1">
                {s.keys.map((k) => (
                  <Kbd key={k} className="h-6 min-w-6 text-xs">
                    {k}
                  </Kbd>
                ))}
              </dd>
            </div>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  )
}
