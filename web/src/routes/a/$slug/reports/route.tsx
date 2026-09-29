import { createFileRoute, Outlet } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader } from '@/components/page-header'
import { useCurrentAccount } from '@/hooks/use-current-account'

export const Route = createFileRoute('/a/$slug/reports')({
  component: ReportsLayout,
})

/** Přehledy a DPH smí jen vlastník, administrátor a účetní (SPEC §7.13); člen uvidí vysvětlení. */
function ReportsLayout() {
  const { role } = useCurrentAccount()
  if (role === 'member') {
    return (
      <>
        <PageHeader title="Přehledy" />
        <PageBody>
          <EmptyState
            icon={LockIcon}
            title="Přehledy nejsou dostupné"
            description="Finanční přehledy a podklady k DPH vidí jen vlastník, administrátor a účetní účtu."
            className="bg-card"
          />
        </PageBody>
      </>
    )
  }
  return <Outlet />
}
