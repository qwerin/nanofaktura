import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { FileQuestionIcon, RotateCwIcon } from 'lucide-react'
import { isApiError } from '@/api/errors'
import { publicQueries } from '@/api/queries/public'
import { LogoMark } from '@/components/logo'
import { publicLabels } from '@/components/public-invoice/i18n'
import { PublicInvoiceView } from '@/components/public-invoice/public-invoice-view'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { todayISO } from '@/lib/date'

// Veřejný odkaz faktury pro zákazníka: bez přihlášení a bez app shellu (SPEC §7.5).
export const Route = createFileRoute('/p/$token')({
  loader: async ({ context, params }) => {
    try {
      return await context.queryClient.ensureQueryData(publicQueries.invoice(params.token))
    } catch {
      return null // chybu (404) vykreslí komponenta
    }
  },
  head: ({ loaderData }) => ({
    meta: [
      { title: loaderData ? `${loaderData.number} · ${loaderData.supplier.name}` : 'Faktura · NanoFaktura' },
      { name: 'robots', content: 'noindex, nofollow' },
      { name: 'referrer', content: 'no-referrer' },
    ],
  }),
  component: PublicInvoicePage,
})

function PublicInvoicePage() {
  const { token } = Route.useParams()
  const invoice = useQuery(publicQueries.invoice(token))

  if (invoice.isPending) return <PublicSkeleton />
  if (invoice.isError) {
    const notFound = isApiError(invoice.error) && invoice.error.status === 404
    return <PublicProblem notFound={notFound} onRetry={() => void invoice.refetch()} />
  }
  return <PublicInvoiceView token={token} invoice={invoice.data} today={todayISO()} />
}

function PublicSkeleton() {
  return (
    <div className="min-h-dvh bg-muted/40" aria-busy="true">
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 pt-6 md:pt-10">
        <div className="flex items-center gap-3">
          <Skeleton className="size-11 rounded-xl" />
          <div className="flex flex-col gap-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-28" />
          </div>
        </div>
        <Skeleton className="h-36 w-full rounded-2xl" />
        <Skeleton className="h-64 w-full rounded-2xl" />
      </div>
    </div>
  )
}

/** Neplatný odkaz / chyba — dvojjazyčně, jazyk dokladu neznáme. */
function PublicProblem({ notFound, onRetry }: { notFound: boolean; onRetry: () => void }) {
  const cs = publicLabels('cs')
  const en = publicLabels('en')
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center bg-muted/40 px-6 py-12 text-center">
      <div className="flex max-w-md flex-col items-center gap-4">
        <span className="flex size-14 items-center justify-center rounded-2xl bg-card shadow-xs ring-1 ring-border">
          <FileQuestionIcon className="size-7 text-muted-foreground" />
        </span>
        <div className="flex flex-col gap-2">
          <h1 className="text-xl font-semibold tracking-tight">{notFound ? cs.notFoundTitle : cs.errorTitle}</h1>
          {notFound && <p className="text-sm text-muted-foreground">{cs.notFoundText}</p>}
        </div>
        <div lang="en" className="flex flex-col gap-1 border-t pt-4 text-sm text-muted-foreground">
          <p className="font-medium text-foreground">{notFound ? en.notFoundTitle : en.errorTitle}</p>
          {notFound && <p>{en.notFoundText}</p>}
        </div>
        {!notFound && (
          <Button onClick={onRetry}>
            <RotateCwIcon data-icon="inline-start" />
            {cs.retry} / {en.retry}
          </Button>
        )}
        <span className="mt-6 inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <LogoMark className="size-4 rounded-[4px]" />
          NanoFaktura
        </span>
      </div>
    </div>
  )
}
