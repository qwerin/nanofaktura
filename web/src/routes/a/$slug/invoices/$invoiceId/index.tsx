import { createFileRoute, Link } from '@tanstack/react-router'
import { PencilIcon } from 'lucide-react'
import { ComingSoon } from '@/components/coming-soon'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/invoices/$invoiceId/')({
  // ID z URL jako číslo; neplatné ID → 404.
  params: {
    parse: ({ invoiceId }) => ({ invoiceId: parseId(invoiceId) }),
    stringify: ({ invoiceId }) => ({ invoiceId: String(invoiceId) }),
  },
  head: () => ({ meta: [{ title: 'Faktura · NanoFaktura' }] }),
  component: InvoiceDetailPage,
})

function InvoiceDetailPage() {
  const { slug, invoiceId } = Route.useParams()
  return (
    <ComingSoon
      title={`Faktura #${invoiceId}`}
      back={{ to: '/a/$slug/invoices', params: { slug } }}
      actions={[
        {
          label: 'Upravit',
          icon: PencilIcon,
          render: <Link to="/a/$slug/invoices/$invoiceId/edit" params={{ slug, invoiceId }} />,
        },
      ]}
    />
  )
}
