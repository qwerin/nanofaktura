import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { priceItemQueries, useUpdatePriceItem } from '@/api/queries/price-items'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { PriceItemForm } from '@/components/price-item/price-item-form'
import { priceItemToFormValues, toUpdatePriceItemInput } from '@/components/price-item/price-item-form-schema'
import { FormSkeleton } from '@/components/skeletons'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/price-items/$priceItemId/edit')({
  params: {
    parse: ({ priceItemId }) => ({ priceItemId: parseId(priceItemId) }),
    stringify: ({ priceItemId }) => ({ priceItemId: String(priceItemId) }),
  },
  loader: ({ context, params }) => context.queryClient.prefetchQuery(priceItemQueries.detail(params.slug, params.priceItemId)),
  head: () => ({ meta: [{ title: 'Upravit položku · NanoFaktura' }] }),
  component: EditPriceItemPage,
})

function EditPriceItemPage() {
  const { slug, priceItemId } = Route.useParams()
  const navigate = useNavigate()
  const item = useQuery(priceItemQueries.detail(slug, priceItemId))
  const update = useUpdatePriceItem(slug, priceItemId)
  const back = { to: '/a/$slug/price-items/$priceItemId', params: { slug, priceItemId } } as const

  if (item.isError) return <PageError error={item.error} reset={() => void item.refetch()} />

  return (
    <>
      <PageHeader title={item.data ? `Upravit ${item.data.name}` : 'Upravit položku'} back={back} />
      <PageBody>
        {item.isPending ? (
          <FormSkeleton />
        ) : (
          <PriceItemForm
            mode="edit"
            defaultValues={priceItemToFormValues(item.data)}
            hasStock={item.data.track_stock}
            onCancel={() => void navigate(back)}
            onSubmit={async (values) => {
              await update.mutateAsync(toUpdatePriceItemInput(values))
              toast.success('Položka uložena')
              await navigate({ ...back, replace: true })
            }}
          />
        )}
      </PageBody>
    </>
  )
}
