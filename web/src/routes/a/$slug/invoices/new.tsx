import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { useMemo } from 'react'
import { z } from 'zod'
import { accountQueries } from '@/api/queries/accounts'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { subjectQueries } from '@/api/queries/subjects'
import { EmptyState } from '@/components/empty-state'
import { InvoiceForm } from '@/components/invoice/invoice-form'
import { documentTypeLabels } from '@/components/invoice/status'
import { useCanEditDocuments } from '@/components/invoice/use-can-edit'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'

const searchSchema = z.object({
  type: z.enum(['invoice', 'proforma', 'correction']).optional().catch(undefined),
  subject_id: z.number().int().positive().optional().catch(undefined),
  related_id: z.number().int().positive().optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/invoices/new')({
  validateSearch: searchSchema,
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Nová faktura · NanoFaktura' }] }),
  component: NewInvoicePage,
})

function NewInvoicePage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const canEdit = useCanEditDocuments()
  const account = useQuery(accountQueries.detail(slug))
  const subject = useQuery({ ...subjectQueries.detail(slug, search.subject_id ?? 0), enabled: Boolean(search.subject_id) })

  const preset = useMemo(
    () => ({
      documentType: search.type,
      relatedId: search.related_id,
      subject: subject.data
        ? { id: subject.data.id, name: subject.data.name, registration_no: subject.data.registration_no, city: subject.data.city }
        : null,
    }),
    [search.type, search.related_id, subject.data],
  )
  const waitingForSubject = Boolean(search.subject_id) && subject.isPending

  const title = search.type && search.type !== 'invoice' ? `Nový ${search.type === 'proforma' ? 'zálohový doklad' : 'opravný doklad'}` : 'Nová faktura'

  return (
    <>
      <PageHeader title={title} description={search.type ? documentTypeLabels[search.type] : undefined} back={{ to: '/a/$slug/invoices', params: { slug } }} />
      <PageBody>
        {!canEdit ? (
          <EmptyState icon={LockIcon} title="Jen pro čtení" description="Vaše role v účtu neumožňuje vystavovat doklady." />
        ) : account.isError ? (
          <PageError error={account.error} reset={() => void account.refetch()} />
        ) : account.data && !waitingForSubject ? (
          <InvoiceForm key={`${slug}-${search.type}-${search.subject_id}`} slug={slug} account={account.data} preset={preset} />
        ) : (
          <FormSkeleton fields={8} className="max-w-3xl" />
        )}
      </PageBody>
    </>
  )
}
