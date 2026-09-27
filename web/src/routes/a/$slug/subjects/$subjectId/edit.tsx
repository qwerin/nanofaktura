import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { subjectQueries } from '@/api/queries/subjects'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'
import { SubjectForm } from '@/components/subject/subject-form'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/subjects/$subjectId/edit')({
  params: {
    parse: ({ subjectId }) => ({ subjectId: parseId(subjectId) }),
    stringify: ({ subjectId }) => ({ subjectId: String(subjectId) }),
  },
  loader: ({ context, params }) =>
    context.queryClient.ensureQueryData(subjectQueries.detail(params.slug, params.subjectId)),
  head: () => ({ meta: [{ title: 'Upravit kontakt · NanoFaktura' }] }),
  component: EditSubjectPage,
})

function EditSubjectPage() {
  const { slug, subjectId } = Route.useParams()
  const navigate = useNavigate()
  const subject = useQuery(subjectQueries.detail(slug, subjectId))
  const toDetail = () =>
    navigate({ to: '/a/$slug/subjects/$subjectId', params: { slug, subjectId }, replace: true })

  return (
    <>
      <PageHeader
        title={subject.data ? `Upravit: ${subject.data.name}` : 'Upravit kontakt'}
        back={{ to: '/a/$slug/subjects/$subjectId', params: { slug, subjectId } }}
      />
      <PageBody>
        {subject.isError ? (
          <PageError error={subject.error} reset={() => void subject.refetch()} />
        ) : subject.data ? (
          <SubjectForm
            key={subject.data.id}
            slug={slug}
            subject={subject.data}
            onCancel={() => void toDetail()}
            onSaved={() => void toDetail()}
          />
        ) : (
          <FormSkeleton fields={8} className="max-w-2xl" />
        )}
      </PageBody>
    </>
  )
}
