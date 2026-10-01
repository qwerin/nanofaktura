import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { accountQueries } from '@/api/queries/accounts'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { invoiceQueries } from '@/api/queries/invoices'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { InvoiceForm } from '@/components/invoice/invoice-form'
import { allowedActions, isSettledProforma } from '@/components/invoice/status'
import { useCanEditDocuments } from '@/components/invoice/use-can-edit'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/invoices/$invoiceId/edit')({
  params: {
    parse: ({ invoiceId }) => ({ invoiceId: parseId(invoiceId) }),
    stringify: ({ invoiceId }) => ({ invoiceId: String(invoiceId) }),
  },
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(invoiceQueries.detail(params.slug, params.invoiceId)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Úprava faktury · NanoFaktura' }] }),
  component: EditInvoicePage,
})

function EditInvoicePage() {
  const { slug, invoiceId } = Route.useParams()
  const canEdit = useCanEditDocuments()
  const invoice = useQuery(invoiceQueries.detail(slug, invoiceId))
  const account = useQuery(accountQueries.detail(slug))
  const back = { to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId } } as const

  const inv = invoice.data
  const editable =
    inv && allowedActions({ ...inv, payment_count: inv.payments.length }).has('edit')

  return (
    <>
      <PageHeader title={inv ? `Upravit ${inv.number}` : 'Úprava faktury'} back={back} />
      <PageBody>
        {invoice.isError || account.isError ? (
          <PageError
            error={invoice.error ?? account.error}
            reset={() => {
              void invoice.refetch()
              void account.refetch()
            }}
          />
        ) : !inv || !account.data ? (
          <FormSkeleton fields={8} className="max-w-3xl" />
        ) : !canEdit || !editable ? (
          <EmptyState
            icon={LockIcon}
            title="Doklad nelze upravit"
            description={
              !canEdit
                ? 'Vaše role v účtu neumožňuje upravovat doklady.'
                : inv.document_type === 'tax_document'
                  ? 'Daňový doklad k přijaté platbě odpovídá platbě zálohy a ručně se neupravuje. Při chybě opravte platbu na zálohové faktuře.'
                  : isSettledProforma(inv)
                    ? 'Záloha je už vyúčtovaná konečnou fakturou. Změny proveďte na ní.'
                    : inv.locked_at
                      ? 'Doklad je zamčený. Odemkněte ho v detailu.'
                      : 'Stornovaný nebo nedobytný doklad nejde upravovat. Nejdřív ho obnovte.'
            }
            action={
              <ButtonLink variant="outline" {...back}>
                Zpět na detail
              </ButtonLink>
            }
          />
        ) : (
          <InvoiceForm key={inv.id} slug={slug} account={account.data} invoice={inv} />
        )}
      </PageBody>
    </>
  )
}
