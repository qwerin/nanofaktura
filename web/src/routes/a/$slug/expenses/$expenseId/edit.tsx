import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { LockIcon } from 'lucide-react'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { expenseQueries, useUpdateExpense } from '@/api/queries/expenses'
import { ExpenseForm } from '@/components/expense/expense-form'
import { expenseToFormValues, toUpdateExpenseInput } from '@/components/expense/expense-form-schema'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { ButtonLink } from '@/components/button-link'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/expenses/$expenseId/edit')({
  params: {
    parse: ({ expenseId }) => ({ expenseId: parseId(expenseId) }),
    stringify: ({ expenseId }) => ({ expenseId: String(expenseId) }),
  },
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(expenseQueries.detail(params.slug, params.expenseId)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Upravit náklad · NanoFaktura' }] }),
  component: EditExpensePage,
})

function EditExpensePage() {
  const { slug, expenseId } = Route.useParams()
  const navigate = useNavigate()
  const expense = useQuery(expenseQueries.detail(slug, expenseId))
  const account = useQuery(accountQueries.detail(slug))
  const update = useUpdateExpense(slug, expenseId)
  const back = { to: '/a/$slug/expenses/$expenseId', params: { slug, expenseId } } as const

  if (expense.isError) return <PageError error={expense.error} reset={() => void expense.refetch()} />

  return (
    <>
      <PageHeader title={expense.data ? `Upravit ${expense.data.number}` : 'Upravit náklad'} back={back} />
      <PageBody>
        {expense.isPending || account.isPending ? (
          <FormSkeleton />
        ) : expense.data.locked_at ? (
          <Alert className="max-w-xl">
            <LockIcon />
            <AlertTitle>Náklad je zamčený</AlertTitle>
            <AlertDescription>
              Zamčený náklad nejde upravovat. Nejdřív ho odemkněte v detailu.
              <ButtonLink {...back} variant="outline" className="mt-3">
                Zpět na detail
              </ButtonLink>
            </AlertDescription>
          </Alert>
        ) : (
          <ExpenseForm
            mode="edit"
            account={account.data}
            defaultValues={expenseToFormValues(expense.data)}
            onCancel={() => void navigate(back)}
            onSubmit={async (values) => {
              await update.mutateAsync(toUpdateExpenseInput(values, account.data?.default_currency || 'CZK'))
              toast.success('Náklad uložen')
              await navigate({ ...back, replace: true })
            }}
          />
        )}
      </PageBody>
    </>
  )
}
