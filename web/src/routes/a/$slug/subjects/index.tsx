import { createFileRoute, Link } from '@tanstack/react-router'
import { UserPlusIcon } from 'lucide-react'
import { z } from 'zod'
import { ComingSoon } from '@/components/coming-soon'

const searchSchema = z.object({
  query: z.string().optional().catch(undefined),
  type: z.enum(['customer', 'supplier', 'both']).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/subjects/')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Kontakty · NanoFaktura' }] }),
  component: SubjectsPage,
})

function SubjectsPage() {
  const { slug } = Route.useParams()
  return (
    <ComingSoon
      title="Kontakty"
      description="Odběratelé a dodavatelé."
      actions={[
        {
          label: 'Nový kontakt',
          icon: UserPlusIcon,
          primary: true,
          render: <Link to="/a/$slug/subjects/new" params={{ slug }} />,
        },
      ]}
    />
  )
}
