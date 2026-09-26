import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/coming-soon'

export const Route = createFileRoute('/a/$slug/invoices/new')({
  head: () => ({ meta: [{ title: 'Nová faktura · NanoFaktura' }] }),
  component: NewInvoicePage,
})

function NewInvoicePage() {
  const { slug } = Route.useParams()
  return <ComingSoon title="Nová faktura" back={{ to: '/a/$slug/invoices', params: { slug } }} />
}
