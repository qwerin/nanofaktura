import { createFileRoute, Link, Outlet } from '@tanstack/react-router'
import { settingsNav } from '@/components/layout/nav'
import { PageHeader } from '@/components/page-header'

export const Route = createFileRoute('/a/$slug/settings')({
  head: () => ({ meta: [{ title: 'Nastavení · NanoFaktura' }] }),
  component: SettingsLayout,
})

function SettingsLayout() {
  const { slug } = Route.useParams()
  return (
    <>
      {/* Desktop: titulek + záložky. Mobil: seznam sekcí na /settings, podstránky mají vlastní hlavičku. */}
      <PageHeader title="Nastavení" show="desktop">
        <nav aria-label="Sekce nastavení" className="-mb-px flex gap-1 overflow-x-auto border-b [scrollbar-width:none]">
          {settingsNav.map((item) => (
            <Link
              key={item.to}
              to={item.to}
              params={{ slug }}
              className="flex h-10 shrink-0 items-center gap-2 border-b-2 border-transparent px-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground data-[status=active]:border-primary data-[status=active]:text-foreground"
            >
              <item.icon className="size-4" />
              {item.label}
            </Link>
          ))}
        </nav>
      </PageHeader>
      <Outlet />
    </>
  )
}
