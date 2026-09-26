import { createFileRoute, notFound, Outlet, redirect } from '@tanstack/react-router'
import { authQueries } from '@/api/queries/auth'
import { AppShell } from '@/components/layout/app-shell'

// Přihlášená část aplikace. Guard: session (/api/auth/me) + členství v účtu `$slug`.
export const Route = createFileRoute('/a/$slug')({
  beforeLoad: async ({ context, params, location }) => {
    const me = await context.queryClient.ensureQueryData(authQueries.me())
    if (!me) {
      throw redirect({ to: '/login', search: { redirect: location.href }, replace: true })
    }
    const membership = me.accounts?.find((a) => a.slug === params.slug)
    if (!membership) {
      // Neznámý / cizí účet → 404 (neprozrazujeme existenci).
      throw notFound()
    }
    return { me, membership }
  },
  component: AccountLayout,
})

function AccountLayout() {
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
