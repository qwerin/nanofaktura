import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { z } from 'zod'
import { PageBody, PageHeader } from '@/components/page-header'
import { SubjectForm } from '@/components/subject/subject-form'

const searchSchema = z.object({
  /** Předvyplněný název (např. z hledání, které nic nenašlo). */
  name: z.string().optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/subjects/new')({
  validateSearch: searchSchema,
  head: () => ({ meta: [{ title: 'Nový kontakt · NanoFaktura' }] }),
  component: NewSubjectPage,
})

function NewSubjectPage() {
  const { slug } = Route.useParams()
  const { name } = Route.useSearch()
  const navigate = useNavigate()
  const back = () => navigate({ to: '/a/$slug/subjects', params: { slug } })

  return (
    <>
      <PageHeader
        title="Nový kontakt"
        description="Zadejte IČO a údaje firmy načteme z ARES."
        back={{ to: '/a/$slug/subjects', params: { slug } }}
      />
      <PageBody>
        <SubjectForm
          slug={slug}
          defaults={name ? { name } : undefined}
          onCancel={() => void back()}
          onSaved={(s) =>
            void navigate({ to: '/a/$slug/subjects/$subjectId', params: { slug, subjectId: s.id }, replace: true })
          }
        />
      </PageBody>
    </>
  )
}
