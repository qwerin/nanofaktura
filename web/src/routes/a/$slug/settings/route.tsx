import { createFileRoute, Link, Outlet } from '@tanstack/react-router'
import { settingsGroups, settingsNav } from '@/components/layout/nav'
import { PageHeader } from '@/components/page-header'

export const Route = createFileRoute('/a/$slug/settings')({
  head: () => ({ meta: [{ title: 'Nastavení · NanoFaktura' }] }),
  component: SettingsLayout,
})

function SettingsLayout() {
  const { slug } = Route.useParams()
  return (
    <>
      {/* Desktop: titulek + svislé menu sekcí vlevo. Mobil: seznam sekcí na /settings, podstránky mají vlastní hlavičku. */}
      <PageHeader title="Nastavení" show="desktop" />
      <div className="md:mx-auto md:flex md:w-full md:max-w-6xl md:gap-8 md:px-8 md:py-6">
        <nav aria-label="Sekce nastavení" className="hidden w-56 shrink-0 md:block">
          <div className="sticky top-20 space-y-5">
            {settingsGroups.map((group) => (
              <div key={group.id}>
                <p className="mb-1 px-3 text-xs font-medium tracking-wide text-muted-foreground uppercase">{group.label}</p>
                <ul className="space-y-0.5">
                  {settingsNav
                    .filter((item) => item.group === group.id)
                    .map((item) => (
                      <li key={item.to}>
                        <Link
                          to={item.to}
                          params={{ slug }}
                          className="flex h-9 items-center gap-2.5 rounded-lg px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground data-[status=active]:bg-primary/10 data-[status=active]:text-primary"
                        >
                          <item.icon className="size-4 shrink-0" />
                          <span className="truncate">{item.label}</span>
                        </Link>
                      </li>
                    ))}
                </ul>
              </div>
            ))}
          </div>
        </nav>
        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </div>
    </>
  )
}
