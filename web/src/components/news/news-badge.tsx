import { SidebarMenuBadge } from '@/components/ui/sidebar'
import { useNews } from '@/hooks/use-news'

/** Upozornění na nepřečtené novinky u položky navigace „Novinky“ (nic, když je vše přečteno). */
export function NewsBadge({ variant = 'pill' }: { variant?: 'sidebar' | 'pill' | 'dot' }) {
  const { unseenCount } = useNews()
  if (!unseenCount) return null
  if (variant === 'dot') {
    return <span aria-label="Nové novinky" className="absolute top-2 right-[calc(50%-14px)] size-2 rounded-full bg-primary ring-2 ring-background" />
  }
  const cls = 'rounded-full bg-primary/10 px-1.5 text-[11px] font-medium text-primary'
  return variant === 'sidebar' ? (
    <SidebarMenuBadge className={`${cls} peer-data-active/menu-button:text-primary`}>Nové</SidebarMenuBadge>
  ) : (
    <span className={`inline-flex h-5 items-center ${cls}`}>Nové</span>
  )
}
