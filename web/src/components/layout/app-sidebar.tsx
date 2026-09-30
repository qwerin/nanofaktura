import { Link, useMatchRoute, useNavigate } from '@tanstack/react-router'
import {
  CheckIcon,
  ChevronsUpDownIcon,
  KeyRoundIcon,
  LogOutIcon,
  PlusIcon,
  SettingsIcon,
  UserIcon,
} from 'lucide-react'
import { useState } from 'react'
import { CreateAccountDialog } from '@/components/account/create-account-dialog'
import { themeOptions, useThemeChoice, type ThemeChoice } from '@/hooks/use-theme-choice'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@/components/ui/sidebar'
import { SidebarSearchButton } from '@/components/search/search-trigger'
import { TodoCountBadge } from '@/components/todos/todo-count-badge'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { roleLabel } from '@/lib/roles'
import { initials, mainNav } from './nav'
import { useLogoutAction } from './use-logout-action'

/** Desktopová navigace (≥ md). Na mobilu ji nahrazuje BottomTabBar. */
export function AppSidebar() {
  const { slug } = useCurrentAccount()
  const matchRoute = useMatchRoute()

  return (
    <Sidebar collapsible="icon" className="hidden md:flex">
      <SidebarHeader>
        <AccountSwitcher />
        <SidebarSearchButton />
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent className="flex flex-col gap-2">
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Nová faktura"
                  render={<Link to="/a/$slug/invoices/new" params={{ slug }} />}
                  className="bg-primary font-medium text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground active:bg-primary/90 active:text-primary-foreground"
                >
                  <PlusIcon />
                  <span>Nová faktura</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
            <SidebarMenu>
              {mainNav.map((item) => (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    tooltip={item.label}
                    isActive={Boolean(matchRoute({ to: item.to, params: { slug }, fuzzy: true }))}
                    render={<Link to={item.to} params={{ slug }} />}
                  >
                    <item.icon />
                    <span>{item.label}</span>
                  </SidebarMenuButton>
                  {item.to === '/a/$slug/todos' && <TodoCountBadge slug={slug} variant="sidebar" />}
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup className="mt-auto">
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  tooltip="Nastavení"
                  isActive={Boolean(matchRoute({ to: '/a/$slug/settings', params: { slug }, fuzzy: true }))}
                  render={<Link to="/a/$slug/settings" params={{ slug }} />}
                >
                  <SettingsIcon />
                  <span>Nastavení</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <UserMenu />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}

function AccountAvatar({ name, className }: { name: string; className?: string }) {
  return (
    <Avatar className={className ?? 'size-8 rounded-lg'}>
      <AvatarFallback className="rounded-lg bg-primary/10 text-xs font-semibold text-primary">
        {initials(name)}
      </AvatarFallback>
    </Avatar>
  )
}

function AccountSwitcher() {
  const { membership, accounts, slug } = useCurrentAccount()
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const name = membership?.name ?? slug

  return (
    <>
      <SidebarMenu>
        <SidebarMenuItem>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <SidebarMenuButton size="lg" className="data-popup-open:bg-sidebar-accent" />
              }
            >
              <AccountAvatar name={name} />
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-semibold">{name}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {roleLabel(membership?.role)}
                </span>
              </div>
              <ChevronsUpDownIcon className="ml-auto text-muted-foreground" />
            </DropdownMenuTrigger>
            <DropdownMenuContent className="min-w-60" align="start" side="bottom" sideOffset={4}>
              <DropdownMenuGroup>
                <DropdownMenuLabel className="text-xs text-muted-foreground">Účty</DropdownMenuLabel>
                {accounts.map((a) => (
                  <DropdownMenuItem
                    key={a.slug}
                    onClick={() => void navigate({ to: '/a/$slug/dashboard', params: { slug: a.slug } })}
                    className="gap-2"
                  >
                    <AccountAvatar name={a.name} className="size-6 rounded-md" />
                    <span className="flex-1 truncate">{a.name}</span>
                    {a.slug === slug && <CheckIcon className="text-primary" />}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setCreateOpen(true)} className="gap-2">
                <PlusIcon />
                Nový účet
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
      </SidebarMenu>
      <CreateAccountDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  )
}

function UserMenu() {
  const { user, slug } = useCurrentAccount()
  const navigate = useNavigate()
  const logout = useLogoutAction()
  const { theme, setTheme } = useThemeChoice()
  if (!user) return null

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<SidebarMenuButton size="lg" className="data-popup-open:bg-sidebar-accent" />}
          >
            <Avatar className="size-8">
              <AvatarFallback className="text-xs font-medium">{initials(user.name || user.email)}</AvatarFallback>
            </Avatar>
            <div className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{user.name}</span>
              <span className="truncate text-xs text-muted-foreground">{user.email}</span>
            </div>
            <ChevronsUpDownIcon className="ml-auto text-muted-foreground" />
          </DropdownMenuTrigger>
          <DropdownMenuContent className="min-w-60" side="top" align="start" sideOffset={4}>
            <DropdownMenuGroup>
              <DropdownMenuItem onClick={() => void navigate({ to: '/a/$slug/settings/profile', params: { slug } })}>
                <UserIcon />
                Můj profil
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => void navigate({ to: '/a/$slug/settings/tokens', params: { slug } })}>
                <KeyRoundIcon />
                API tokeny
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel className="text-xs text-muted-foreground">Motiv</DropdownMenuLabel>
              <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as ThemeChoice)}>
                {themeOptions.map((o) => (
                  <DropdownMenuRadioItem key={o.value} value={o.value}>
                    <o.icon />
                    {o.label}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={logout.run} disabled={logout.isPending}>
              <LogOutIcon />
              Odhlásit se
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}

export { AccountAvatar }
