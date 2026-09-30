import { Link, type LinkOptions } from '@tanstack/react-router'
import { ChevronLeftIcon, EllipsisIcon, type LucideIcon } from 'lucide-react'
import { useState, type ReactElement, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { MobileSearchButton } from '@/components/search/search-trigger'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

export interface PageAction {
  label: string
  icon?: LucideIcon
  onClick?: () => void
  /** Místo tlačítka vykreslit odkaz, např. `<Link to="…" />` nebo `<a href target="_blank" />`. */
  render?: ReactElement
  variant?: 'default' | 'outline' | 'secondary' | 'ghost' | 'destructive'
  /** Na mobilu zůstane v hlavičce jako ikonové tlačítko (vyžaduje `icon`); ostatní jdou do menu. */
  primary?: boolean
  /** Na desktopu schovat do menu „…“ (vedle ostatních tlačítek). */
  overflow?: boolean
  disabled?: boolean
}

interface PageHeaderProps {
  title: ReactNode
  description?: ReactNode
  /** Kam vede šipka zpět na mobilu, např. `{ to: '/a/$slug/invoices', params: { slug } }`. */
  back?: LinkOptions
  actions?: PageAction[]
  /** Libovolný obsah pod titulkem (záložky, filtry…). */
  children?: ReactNode
  /** Zobrazit jen na mobilu / jen na desktopu (např. vnořené stránky nastavení). */
  show?: 'both' | 'mobile' | 'desktop'
  className?: string
}

/**
 * Hlavička stránky.
 * - mobil: kompaktní sticky lišta — zpět, titulek, primární akce jako ikony, ostatní v Drawer menu („…“).
 * - desktop: velký titulek + popis, akce jako tlačítka vpravo, přepínač sidebaru vlevo.
 */
export function PageHeader({
  title,
  description,
  back,
  actions = [],
  children,
  show = 'both',
  className,
}: PageHeaderProps) {
  const [menuOpen, setMenuOpen] = useState(false)
  const mobileInline = actions.filter((a) => a.primary && a.icon)
  const mobileMenu = actions.filter((a) => !(a.primary && a.icon))

  return (
    <header
      className={cn(
        'sticky top-0 z-30 border-b bg-background/85 pt-safe backdrop-blur-md supports-backdrop-filter:bg-background/70',
        'md:static md:border-b-0 md:bg-transparent md:pt-0 md:backdrop-blur-none',
        show === 'mobile' && 'md:hidden',
        show === 'desktop' && 'max-md:hidden',
        className,
      )}
    >
      {/* Mobil */}
      <div className="flex h-14 items-center gap-1 px-2 md:hidden">
        {back ? (
          <Button
            variant="ghost"
            size="icon"
            aria-label="Zpět"
            nativeButton={false}
            render={<Link {...back} />}
          >
            <ChevronLeftIcon className="size-6" />
          </Button>
        ) : (
          <span className="w-2" />
        )}
        <h1 className="min-w-0 flex-1 truncate text-lg font-semibold tracking-tight">{title}</h1>
        {/* Hledání na hlavních stránkách (bez šipky zpět); podstránky mají místo pro vlastní akce. */}
        {!back && <MobileSearchButton />}
        {mobileInline.map((a) => (
          <ActionButton key={a.label} action={a} iconOnly />
        ))}
        {mobileMenu.length > 0 && (
          <>
            <Button variant="ghost" size="icon" aria-label="Další akce" onClick={() => setMenuOpen(true)}>
              <EllipsisIcon className="size-5" />
            </Button>
            <Drawer open={menuOpen} onOpenChange={setMenuOpen} showSwipeHandle>
              <DrawerContent>
                <DrawerHeader className="group-data-[swipe-axis=y]/drawer-popup:text-left">
                  <DrawerTitle>Akce</DrawerTitle>
                </DrawerHeader>
                <div className="flex flex-col gap-1 p-2 pb-[max(1rem,env(safe-area-inset-bottom))]">
                  {mobileMenu.map((a) => (
                    <DrawerActionRow key={a.label} action={a} onDone={() => setMenuOpen(false)} />
                  ))}
                </div>
              </DrawerContent>
            </Drawer>
          </>
        )}
      </div>

      {/* Desktop */}
      <div className="mx-auto hidden w-full max-w-6xl items-start gap-3 px-8 pt-6 md:flex">
        <SidebarTrigger className="-ml-2 mt-0.5 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-2xl font-semibold tracking-tight">{title}</h1>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions.length > 0 && (
          <div className="flex shrink-0 items-center gap-2">
            {actions
              .filter((a) => !a.overflow)
              .map((a) => (
                <ActionButton key={a.label} action={a} />
              ))}
            {actions.some((a) => a.overflow) && <OverflowMenu actions={actions.filter((a) => a.overflow)} />}
          </div>
        )}
      </div>

      {children && <div className="mx-auto w-full max-w-6xl px-4 pb-2 md:px-8 md:pt-4">{children}</div>}
    </header>
  )
}

function ActionButton({ action, iconOnly }: { action: PageAction; iconOnly?: boolean }) {
  const Icon = action.icon
  const variant = action.variant ?? (action.primary ? 'default' : 'outline')
  return (
    <Button
      variant={iconOnly ? 'ghost' : variant}
      size={iconOnly ? 'icon' : 'default'}
      aria-label={iconOnly ? action.label : undefined}
      onClick={action.onClick}
      disabled={action.disabled}
      nativeButton={!action.render}
      render={action.render}
      className={cn(iconOnly && action.primary && 'text-primary')}
    >
      {Icon && <Icon data-icon="inline-start" className={cn(iconOnly && 'size-5')} />}
      {!iconOnly && action.label}
    </Button>
  )
}

function OverflowMenu({ actions }: { actions: PageAction[] }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="outline" size="icon" aria-label="Další akce" />}>
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-auto min-w-52">
        {actions.map((a) => {
          const Icon = a.icon
          return (
            <DropdownMenuItem
              key={a.label}
              variant={a.variant === 'destructive' ? 'destructive' : 'default'}
              disabled={a.disabled}
              onClick={a.onClick}
              render={a.render}
              className="py-1.5"
            >
              {Icon && <Icon />}
              {a.label}
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function DrawerActionRow({ action, onDone }: { action: PageAction; onDone: () => void }) {
  const Icon = action.icon
  return (
    <Button
      variant="ghost"
      className={cn(
        'h-12 justify-start gap-3 px-3 text-base font-normal',
        action.variant === 'destructive' && 'text-destructive hover:text-destructive',
      )}
      disabled={action.disabled}
      nativeButton={!action.render}
      render={action.render}
      onClick={() => {
        action.onClick?.()
        onDone()
      }}
    >
      {Icon && <Icon className="size-5 text-muted-foreground" />}
      {action.label}
    </Button>
  )
}

/** Obsahová část stránky se sjednocenou šířkou a okraji. */
export function PageBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('mx-auto w-full max-w-6xl px-4 py-4 md:px-8 md:py-6', className)}>{children}</div>
}
