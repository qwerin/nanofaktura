import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/coming-soon'

export const Route = createFileRoute('/a/$slug/dashboard')({
  head: () => ({ meta: [{ title: 'Přehled · NanoFaktura' }] }),
  component: () => <ComingSoon title="Přehled" description="Tržby, neuhrazené a faktury po splatnosti." />,
})
