import { createFileRoute, notFound } from '@tanstack/react-router'
import { z } from 'zod'
import { adminQueries } from '@/api/queries/admin'
import { EmailTestPanel } from '@/components/admin/email-test-panel'
import { InstanceStatusPanel } from '@/components/admin/instance-status-panel'
import { UsersPanel } from '@/components/admin/users-panel'
import { PageBody, PageHeader } from '@/components/page-header'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'

const tabs = [
  { value: 'status', label: 'Stav instance' },
  { value: 'email', label: 'Test e-mailu' },
  { value: 'users', label: 'Uživatelé' },
] as const
type Tab = (typeof tabs)[number]['value']

const searchSchema = z.object({ tab: z.enum(['status', 'email', 'users']).optional() })

// Správa instance (SPEC §3.2): jen pro správce instance (NANOFAKTURA_ADMIN_EMAILS + ověřený e-mail).
export const Route = createFileRoute('/a/$slug/admin')({
  validateSearch: searchSchema,
  beforeLoad: ({ context }) => {
    if (!context.me.instance_admin) throw notFound()
  },
  loader: ({ context }) => context.queryClient.prefetchQuery(adminQueries.status()),
  head: () => ({ meta: [{ title: 'Správa instance · NanoFaktura' }] }),
  component: AdminPage,
})

function AdminPage() {
  const { slug } = Route.useParams()
  const { tab = 'status' } = Route.useSearch()
  const navigate = Route.useNavigate()
  const { user } = useCurrentAccount()

  return (
    <>
      <PageHeader
        title="Správa instance"
        description="Stav serveru, nastavení odesílání e-mailů a uživatelé celé instance NanoFaktury."
        back={{ to: '/a/$slug/dashboard', params: { slug } }}
      />
      <PageBody className="max-w-5xl">
        <Tabs
          value={tab}
          onValueChange={(v) => void navigate({ search: { tab: v === 'status' ? undefined : (v as Tab) }, replace: true })}
          className="gap-5"
        >
          <TabsList className="h-11! w-full sm:w-auto md:h-9!">
            {tabs.map((t) => (
              <TabsTrigger key={t.value} value={t.value} className="px-3">
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="status">
            <InstanceStatusPanel />
          </TabsContent>
          <TabsContent value="email">
            <EmailTestPanel defaultTo={user?.email ?? ''} />
          </TabsContent>
          <TabsContent value="users">
            <UsersPanel />
          </TabsContent>
        </Tabs>
      </PageBody>
    </>
  )
}
