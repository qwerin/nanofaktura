import { createFileRoute, notFound, redirect } from '@tanstack/react-router'
import { authQueries } from '@/api/queries/auth'

// /admin → Správa instance v rozvržení prvního účtu (navigace aplikace zůstane).
export const Route = createFileRoute('/admin')({
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(authQueries.me())
    if (!me) throw redirect({ to: '/login', search: { redirect: location.href }, replace: true })
    const first = me.accounts?.[0]
    if (!me.instance_admin || !first) throw notFound()
    throw redirect({ to: '/a/$slug/admin', params: { slug: first.slug }, replace: true })
  },
})
