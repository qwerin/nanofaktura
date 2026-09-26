import { createFileRoute, Link } from '@tanstack/react-router'
import { PlusIcon } from 'lucide-react'
import { z } from 'zod'
import { ComingSoon } from '@/components/coming-soon'

// Filtry seznamu žijí v URL (typované search params). `.catch()` = neplatná hodnota se tiše zahodí.
const searchSchema = z.object({
  status: z.enum(['open', 'sent', 'paid', 'overdue', 'cancelled', 'uncollectible']).optional().catch(undefined),
  document_type: z.enum(['invoice', 'proforma', 'correction']).optional().catch(undefined),
  query: z.string().optional().catch(undefined),
  page: z.number().int().min(1).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/invoices/')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Faktury · NanoFaktura' }] }),
  component: InvoicesPage,
})

function InvoicesPage() {
  const { slug } = Route.useParams()
  return (
    <ComingSoon
      title="Faktury"
      description="Vydané faktury, zálohy a dobropisy."
      actions={[
        {
          label: 'Nová faktura',
          icon: PlusIcon,
          primary: true,
          render: <Link to="/a/$slug/invoices/new" params={{ slug }} />,
        },
      ]}
    />
  )
}
