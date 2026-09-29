import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { z } from 'zod'
import { accountQueries } from '@/api/queries/accounts'
import { templateQueries } from '@/api/queries/templates'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { RecurringForm } from '@/components/recurring/recurring-form'
import { FormSkeleton } from '@/components/skeletons'
import { useCurrentAccount } from '@/hooks/use-current-account'

const searchSchema = z.object({
  template_id: z.number().int().positive().optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/recurring/new')({
  validateSearch: searchSchema,
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(templateQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Nová pravidelná faktura · NanoFaktura' }] }),
  component: NewRecurringPage,
})

function NewRecurringPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = useNavigate()
  const { canEdit } = useCurrentAccount()
  const account = useQuery(accountQueries.detail(slug))

  return (
    <>
      <PageHeader
        title="Nová pravidelná faktura"
        description="Vyberte šablonu a jak často se má faktura vystavovat."
        back={{ to: '/a/$slug/recurring', params: { slug } }}
      />
      <PageBody>
        {!canEdit ? (
          <EmptyState icon={LockIcon} title="Jen pro čtení" description="Vaše role v účtu neumožňuje zakládat pravidelné faktury." />
        ) : account.isError ? (
          <PageError error={account.error} reset={() => void account.refetch()} />
        ) : account.data ? (
          <RecurringForm
            key={`${slug}-${search.template_id}`}
            slug={slug}
            account={account.data}
            presetTemplateId={search.template_id}
            onSaved={(r) => void navigate({ to: '/a/$slug/recurring/$recurringId', params: { slug, recurringId: r.id }, replace: true })}
          />
        ) : (
          <FormSkeleton fields={6} className="max-w-3xl" />
        )}
      </PageBody>
    </>
  )
}
