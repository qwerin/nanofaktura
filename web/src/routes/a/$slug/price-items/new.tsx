import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { useCreatePriceItem } from '@/api/queries/price-items'
import { PageBody, PageHeader } from '@/components/page-header'
import { PriceItemForm } from '@/components/price-item/price-item-form'
import { newPriceItemDefaults, toCreatePriceItemInput } from '@/components/price-item/price-item-form-schema'
import { FormSkeleton } from '@/components/skeletons'

export const Route = createFileRoute('/a/$slug/price-items/new')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'Nová položka ceníku · NanoFaktura' }] }),
  component: NewPriceItemPage,
})

function NewPriceItemPage() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const account = useQuery(accountQueries.detail(slug))
  const create = useCreatePriceItem(slug)

  return (
    <>
      <PageHeader title="Nová položka" back={{ to: '/a/$slug/price-items', params: { slug } }} />
      <PageBody>
        {account.isPending ? (
          <FormSkeleton />
        ) : (
          <PriceItemForm
            mode="create"
            defaultValues={newPriceItemDefaults(account.data)}
            onCancel={() => void navigate({ to: '/a/$slug/price-items', params: { slug } })}
            onSubmit={async (values) => {
              const item = await create.mutateAsync(toCreatePriceItemInput(values))
              toast.success('Položka přidána do ceníku')
              await navigate({ to: '/a/$slug/price-items/$priceItemId', params: { slug, priceItemId: item.id }, replace: true })
            }}
          />
        )}
      </PageBody>
    </>
  )
}
