import { createFileRoute, notFound, Outlet, redirect, useMatch } from '@tanstack/react-router'
import { accountQueries } from '@/api/queries/accounts'
import { authQueries } from '@/api/queries/auth'
import { AppShell } from '@/components/layout/app-shell'
import { isOnboarded, isProfileEmpty, markOnboarded } from '@/lib/onboarding'
import { canManageSettingsRole } from '@/lib/roles'

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

    // Průvodce po registraci: jednou, jen pro správce a jen když je firemní profil prázdný.
    const onboardingPath = `/a/${params.slug}/onboarding`
    if (location.pathname !== onboardingPath && canManageSettingsRole(membership.role)) {
      const account = await context.queryClient.ensureQueryData(accountQueries.detail(params.slug))
      if (!isOnboarded(account)) {
        if (isProfileEmpty(account)) {
          throw redirect({ to: '/a/$slug/onboarding', params: { slug: params.slug }, replace: true })
        }
        // Vyplněný profil → průvodce už nikdy nenabízet.
        void markOnboarded(context.queryClient, params.slug)
      }
    }
    return { me, membership }
  },
  component: AccountLayout,
})

function AccountLayout() {
  // Průvodce běží na celou obrazovku bez navigace.
  const onboarding = useMatch({ from: '/a/$slug/onboarding', shouldThrow: false })
  if (onboarding) return <Outlet />
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  )
}
