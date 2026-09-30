import { createFileRoute, Link, Navigate } from '@tanstack/react-router'
import { ChevronRightIcon } from 'lucide-react'
import { settingsGroups, settingsNav } from '@/components/layout/nav'
import { PageBody, PageHeader } from '@/components/page-header'
import { useIsMobile } from '@/hooks/use-mobile'

export const Route = createFileRoute('/a/$slug/settings/')({
  component: SettingsIndex,
})

/** Mobil: seznam sekcí nastavení. Desktop: rovnou první záložka. */
function SettingsIndex() {
  const { slug } = Route.useParams()
  const isMobile = useIsMobile()
  if (!isMobile) return <Navigate to="/a/$slug/settings/company" params={{ slug }} replace />

  return (
    <>
      <PageHeader title="Nastavení" />
      <PageBody>
        <div className="space-y-6">
          {settingsGroups.map((group) => (
            <section key={group.id}>
              <h2 className="mb-2 px-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">{group.label}</h2>
              <ul className="overflow-hidden rounded-xl border bg-card">
                {settingsNav
                  .filter((item) => item.group === group.id)
                  .map((item) => (
                    <li key={item.to} className="border-b last:border-b-0">
                      <Link
                        to={item.to}
                        params={{ slug }}
                        className="flex min-h-16 items-center gap-3 px-4 py-3 active:bg-muted"
                      >
                        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
                          <item.icon className="size-5 text-muted-foreground" />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="font-medium">{item.label}</span>
                          <span className="truncate text-sm text-muted-foreground">{item.description}</span>
                        </span>
                        <ChevronRightIcon className="size-4 text-muted-foreground" />
                      </Link>
                    </li>
                  ))}
              </ul>
            </section>
          ))}
        </div>
      </PageBody>
    </>
  )
}
