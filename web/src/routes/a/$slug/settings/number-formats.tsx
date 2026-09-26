import { createFileRoute } from '@tanstack/react-router'
import { ConstructionIcon } from 'lucide-react'
import { EmptyState } from '@/components/empty-state'
import { SettingsPage } from '@/components/settings-page'

export const Route = createFileRoute('/a/$slug/settings/number-formats')({
  component: () => (
    <SettingsPage title="Číselné řady" description="Formát čísel faktur, záloh a dobropisů, např. {YYYY}-{NNNN} → 2026-0001.">
      <EmptyState icon={ConstructionIcon} title="Připravujeme" description="Tato část aplikace se právě staví." />
    </SettingsPage>
  ),
})
