import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/coming-soon'

export const Route = createFileRoute('/a/$slug/subjects/new')({
  head: () => ({ meta: [{ title: 'Nový kontakt · NanoFaktura' }] }),
  component: NewSubjectPage,
})

function NewSubjectPage() {
  const { slug } = Route.useParams()
  return <ComingSoon title="Nový kontakt" back={{ to: '/a/$slug/subjects', params: { slug } }} />
}
