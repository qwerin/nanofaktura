import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/errors'
import { accountQueries } from '@/api/queries/accounts'
import { uploadAttachment } from '@/api/queries/attachments'
import { useCreateExpense } from '@/api/queries/expenses'
import { PendingFiles } from '@/components/attachments/pending-files'
import { ExpenseForm } from '@/components/expense/expense-form'
import { newExpenseDefaults, toCreateExpenseInput } from '@/components/expense/expense-form-schema'
import { PageBody, PageHeader } from '@/components/page-header'
import { FormSkeleton } from '@/components/skeletons'

export const Route = createFileRoute('/a/$slug/expenses/new')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'Nový náklad · NanoFaktura' }] }),
  component: NewExpensePage,
})

function NewExpensePage() {
  const { slug } = Route.useParams()
  const navigate = useNavigate()
  const account = useQuery(accountQueries.detail(slug))
  const create = useCreateExpense(slug)
  const [files, setFiles] = useState<File[]>([])

  return (
    <>
      <PageHeader title="Nový náklad" back={{ to: '/a/$slug/expenses', params: { slug } }} />
      <PageBody>
        {account.isPending ? (
          <FormSkeleton />
        ) : (
          <ExpenseForm
            mode="create"
            account={account.data}
            defaultValues={newExpenseDefaults(account.data)}
            submitLabel="Uložit náklad"
            onCancel={() => void navigate({ to: '/a/$slug/expenses', params: { slug } })}
            top={
              <section className="mb-6 flex flex-col gap-3 rounded-xl border border-dashed bg-muted/20 p-4 md:mb-8 md:flex-row md:items-start md:gap-10">
                <div className="md:w-52 md:shrink-0">
                  <h2 className="text-base font-semibold tracking-tight">Doklad</h2>
                  <p className="mt-1 text-sm text-muted-foreground">Vyfoťte účtenku nebo přiložte PDF faktury.</p>
                </div>
                <PendingFiles files={files} onChange={setFiles} hint="" className="min-w-0 flex-1" />
              </section>
            }
            onSubmit={async (values) => {
              const expense = await create.mutateAsync(toCreateExpenseInput(values, account.data?.default_currency || 'CZK'))
              // Náklad je uložený; přílohy nahráváme postupně a chyby jen hlásíme (dají se nahrát i z detailu).
              let failed = 0
              for (const file of files) {
                try {
                  await uploadAttachment(slug, { ownerType: 'expense', ownerId: expense.id, file })
                } catch (err) {
                  failed++
                  toast.error(`Přílohu „${file.name}“ se nepodařilo nahrát: ${errorMessage(err)}`)
                }
              }
              toast.success(failed ? 'Náklad uložen, některé přílohy chybí' : `Náklad ${expense.number} uložen`)
              await navigate({ to: '/a/$slug/expenses/$expenseId', params: { slug, expenseId: expense.id }, replace: true })
            }}
          />
        )}
      </PageBody>
    </>
  )
}
