import { createFileRoute } from '@tanstack/react-router'
import { ConstructionIcon } from 'lucide-react'
import { EmptyState } from '@/components/empty-state'
import { SettingsPage } from '@/components/settings-page'

export const Route = createFileRoute('/a/$slug/settings/bank-accounts')({
  component: () => (
    <SettingsPage title="Bankovní účty" description="Účty, na které vám odběratelé platí. Výchozí účet pro každou měnu se předvyplní na fakturu.">
      <EmptyState icon={ConstructionIcon} title="Připravujeme" description="Tato část aplikace se právě staví." />
    </SettingsPage>
  ),
})
