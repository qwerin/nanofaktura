import type { ReactNode } from 'react'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { AppSidebar } from './app-sidebar'
import { BottomTabBar } from './bottom-tab-bar'

function readSidebarCookie(): boolean {
  const m = /(?:^|;\s*)sidebar_state=(true|false)/.exec(document.cookie)
  return m ? m[1] === 'true' : true
}

/**
 * Rozvržení přihlášené části:
 * - ≥ md: sbalitelný Sidebar vlevo (stav v cookie `sidebar_state`, zkratka Ctrl/⌘+B)
 * - < md: obsah na celou šířku + spodní tab bar
 */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <SidebarProvider defaultOpen={readSidebarCookie()}>
      <AppSidebar />
      <SidebarInset className="min-w-0 pb-[calc(var(--spacing-tabbar)+env(safe-area-inset-bottom))] md:pb-0">
        {children}
      </SidebarInset>
      <BottomTabBar />
    </SidebarProvider>
  )
}
