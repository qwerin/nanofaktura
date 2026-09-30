import { SearchIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import { isMacPlatform } from '@/lib/shortcuts'
import { Kbd } from './command-palette'
import { useCommandPalette } from './palette-context'

/** „Hledat… ⌘K“ v hlavičce sidebaru (sbalený sidebar → jen ikona s tooltipem). */
export function SidebarSearchButton() {
  const palette = useCommandPalette()
  if (!palette) return null
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          tooltip="Hledat (⌘K)"
          onClick={() => palette.open()}
          className="border bg-background text-muted-foreground shadow-xs hover:text-foreground"
        >
          <SearchIcon />
          <span className="flex-1">Hledat…</span>
          <Kbd className="group-data-[collapsible=icon]:hidden">{isMacPlatform() ? '⌘K' : 'Ctrl K'}</Kbd>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}

/** Ikona lupy do mobilní hlavičky stránky → paleta přes celou obrazovku. */
export function MobileSearchButton() {
  const palette = useCommandPalette()
  if (!palette) return null
  return (
    <Button variant="ghost" size="icon" aria-label="Hledat" onClick={() => palette.open()}>
      <SearchIcon className="size-5" />
    </Button>
  )
}
