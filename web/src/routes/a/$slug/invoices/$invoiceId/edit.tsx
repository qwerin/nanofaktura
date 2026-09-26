import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/coming-soon'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/invoices/$invoiceId/edit')({
  params: {
    parse: ({ invoiceId }) => ({ invoiceId: parseId(invoiceId) }),
    stringify: ({ invoiceId }) => ({ invoiceId: String(invoiceId) }),
  },
  head: () => ({ meta: [{ title: 'Úprava faktury · NanoFaktura' }] }),
  component: EditInvoicePage,
})

function EditInvoicePage() {
  const { slug, invoiceId } = Route.useParams()
  return (
    <ComingSoon
      title="Úprava faktury"
      back={{ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId } }}
    />
  )
}
