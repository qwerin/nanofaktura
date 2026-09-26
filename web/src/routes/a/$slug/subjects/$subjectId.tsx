import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/coming-soon'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/subjects/$subjectId')({
  params: {
    parse: ({ subjectId }) => ({ subjectId: parseId(subjectId) }),
    stringify: ({ subjectId }) => ({ subjectId: String(subjectId) }),
  },
  head: () => ({ meta: [{ title: 'Kontakt · NanoFaktura' }] }),
  component: SubjectDetailPage,
})

function SubjectDetailPage() {
  const { slug, subjectId } = Route.useParams()
  return <ComingSoon title={`Kontakt #${subjectId}`} back={{ to: '/a/$slug/subjects', params: { slug } }} />
}
