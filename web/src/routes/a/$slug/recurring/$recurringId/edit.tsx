import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { accountQueries } from '@/api/queries/accounts'
import { recurringQueries } from '@/api/queries/recurring'
import { templateQueries } from '@/api/queries/templates'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { RecurringForm } from '@/components/recurring/recurring-form'
import { FormSkeleton } from '@/components/skeletons'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/recurring/$recurringId/edit')({
  params: {
    parse: ({ recurringId }) => ({ recurringId: parseId(recurringId) }),
    stringify: ({ recurringId }) => ({ recurringId: String(recurringId) }),
  },
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(recurringQueries.detail(params.slug, params.recurringId)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(templateQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Upravit pravidelnou fakturu · NanoFaktura' }] }),
  component: EditRecurringPage,
})

function EditRecurringPage() {
  const { slug, recurringId } = Route.useParams()
  const navigate = useNavigate()
  const { canEdit } = useCurrentAccount()
  const recurring = useQuery(recurringQueries.detail(slug, recurringId))
  const account = useQuery(accountQueries.detail(slug))
  const back = { to: '/a/$slug/recurring/$recurringId', params: { slug, recurringId } } as const
  const error = recurring.error ?? account.error

  return (
    <>
      <PageHeader title={recurring.data ? `Upravit: ${recurring.data.name}` : 'Upravit'} back={back} />
      <PageBody>
        {!canEdit ? (
          <EmptyState icon={LockIcon} title="Jen pro čtení" description="Vaše role v účtu neumožňuje upravovat pravidelné faktury." />
        ) : error ? (
          <PageError error={error} reset={() => void Promise.all([recurring.refetch(), account.refetch()])} />
        ) : recurring.data && account.data ? (
          <RecurringForm
            key={recurring.data.id}
            slug={slug}
            account={account.data}
            recurring={recurring.data}
            onSaved={() => void navigate({ ...back, replace: true })}
          />
        ) : (
          <FormSkeleton fields={6} className="max-w-3xl" />
        )}
      </PageBody>
    </>
  )
}
