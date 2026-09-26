import { createFileRoute, redirect } from '@tanstack/react-router'
import { BuildingIcon } from 'lucide-react'
import { useState } from 'react'
import { authQueries } from '@/api/queries/auth'
import { CreateAccountDialog } from '@/components/account/create-account-dialog'
import { EmptyState } from '@/components/empty-state'
import { AuthLayout } from '@/components/auth-layout'
import { Button } from '@/components/ui/button'

// `/` → první účet uživatele, nebo přihlášení.
export const Route = createFileRoute('/')({
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(authQueries.me())
    if (!me) throw redirect({ to: '/login', search: { redirect: location.href }, replace: true })
    const first = me.accounts?.[0]
    if (first) throw redirect({ to: '/a/$slug/dashboard', params: { slug: first.slug }, replace: true })
  },
  component: NoAccount,
})

/** Uživatel bez jediného účtu (např. mu byl odebrán přístup) — nabídneme založení. */
function NoAccount() {
  const [open, setOpen] = useState(false)
  return (
    <AuthLayout>
      <EmptyState
        icon={BuildingIcon}
        title="Zatím nemáte žádný účet"
        description="Založte účet firmy nebo OSVČ, za kterou budete vystavovat faktury."
        action={<Button onClick={() => setOpen(true)}>Založit účet</Button>}
      />
      <CreateAccountDialog open={open} onOpenChange={setOpen} />
    </AuthLayout>
  )
}
