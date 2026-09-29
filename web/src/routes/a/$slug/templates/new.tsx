import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { useMemo } from 'react'
import { z } from 'zod'
import { accountQueries } from '@/api/queries/accounts'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { subjectQueries } from '@/api/queries/subjects'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { TemplateForm } from '@/components/recurring/template-form'
import { FormSkeleton } from '@/components/skeletons'
import { useCurrentAccount } from '@/hooks/use-current-account'

const searchSchema = z.object({
  subject_id: z.number().int().positive().optional().catch(undefined),
  /** Po uložení pokračovat v založení pravidelné faktury. */
  next: z.enum(['recurring']).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/templates/new')({
  validateSearch: searchSchema,
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Nová šablona · NanoFaktura' }] }),
  component: NewTemplatePage,
})

function NewTemplatePage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate()
  const { canEdit } = useCurrentAccount()
  const account = useQuery(accountQueries.detail(slug))
  const subject = useQuery({ ...subjectQueries.detail(slug, search.subject_id ?? 0), enabled: Boolean(search.subject_id) })
  const preset = useMemo(
    () =>
      subject.data
        ? { id: subject.data.id, name: subject.data.name, registration_no: subject.data.registration_no, city: subject.data.city }
        : undefined,
    [subject.data],
  )
  const waiting = Boolean(search.subject_id) && subject.isPending
  const back =
    search.next === 'recurring'
      ? ({ to: '/a/$slug/recurring/new', params: { slug } } as const)
      : ({ to: '/a/$slug/templates', params: { slug } } as const)

  return (
    <>
      <PageHeader
        title="Nová šablona"
        description={search.next === 'recurring' ? 'Po uložení budete pokračovat nastavením opakování.' : 'Předvyplněná faktura pro opakované vystavování.'}
        back={back}
      />
      <PageBody>
        {!canEdit ? (
          <EmptyState icon={LockIcon} title="Jen pro čtení" description="Vaše role v účtu neumožňuje zakládat šablony." />
        ) : account.isError ? (
          <PageError error={account.error} reset={() => void account.refetch()} />
        ) : account.data && !waiting ? (
          <TemplateForm
            key={`${slug}-${search.subject_id}`}
            slug={slug}
            account={account.data}
            subject={preset}
            onSaved={(t) => {
              if (search.next === 'recurring') {
                void navigate({ to: '/a/$slug/recurring/new', params: { slug }, search: { template_id: t.id }, replace: true })
              } else {
                void navigate({ to: '/a/$slug/templates/$templateId', params: { slug, templateId: t.id }, replace: true })
              }
            }}
          />
        ) : (
          <FormSkeleton fields={8} className="max-w-3xl" />
        )}
      </PageBody>
    </>
  )
}
