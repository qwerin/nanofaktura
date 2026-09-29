import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { CalendarClockIcon, MailIcon, PlusIcon, RepeatIcon, TriangleAlertIcon } from 'lucide-react'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { recurringQueries, useSetRecurringActive } from '@/api/queries/recurring'
import { templateQueries } from '@/api/queries/templates'
import type { Account, InvoiceTemplate, Recurring } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { periodLabel } from '@/components/recurring/period'
import { RecurringTabs } from '@/components/recurring/recurring-tabs'
import { templateTotal } from '@/components/recurring/template-model'
import { useSubjectMap } from '@/components/recurring/use-subject-map'
import { ResponsiveList } from '@/components/responsive-list'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate, formatDueRelative, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/recurring/')({
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(recurringQueries.list(params.slug)),
      context.queryClient.prefetchQuery(templateQueries.list(params.slug)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Pravidelné faktury · NanoFaktura' }] }),
  component: RecurringPage,
})

function RecurringPage() {
  const { slug } = Route.useParams()
  const { canEdit } = useCurrentAccount()
  const navigate = useNavigate()
  const list = useQuery(recurringQueries.list(slug))
  const templates = useQuery(templateQueries.list(slug))
  const account = useQuery(accountQueries.detail(slug))
  const setActive = useSetRecurringActive(slug)

  const byId = new Map((templates.data?.items ?? []).map((t) => [t.id, t]))
  const subjects = useSubjectMap(slug, [...byId.values()].map((t) => t.subject_id))
  // Aktivní první, pak podle příštího vystavení.
  const items = list.data?.items
    ? [...list.data.items].sort((a, b) => Number(b.active) - Number(a.active) || a.next_occurrence_on.localeCompare(b.next_occurrence_on))
    : undefined

  const info = (r: Recurring) => {
    const t = byId.get(r.template_id)
    return { template: t, client: t ? (subjects.get(t.subject_id)?.name ?? '…') : '…', amount: amountText(t, account.data) }
  }
  const toggle = (r: Recurring, active: boolean) =>
    setActive.mutate(
      { id: r.id, active },
      { onSuccess: (res) => toast.success(active ? `Zapnuto · příští vystavení ${formatDate(res.next_occurrence_on)}` : 'Vypnuto') },
    )
  const open = (r: Recurring) => navigate({ to: '/a/$slug/recurring/$recurringId', params: { slug, recurringId: r.id } })

  const activeSwitch = (r: Recurring) => (
    // Klik na přepínač nesmí otevřít detail (řádek/karta je klikací).
    <span onClick={(e) => e.stopPropagation()} onKeyDown={(e) => e.stopPropagation()} className="inline-flex">
      <Switch
        checked={r.active}
        disabled={!canEdit || setActive.isPending}
        onCheckedChange={(v) => toggle(r, v)}
        aria-label={r.active ? `Vypnout ${r.name}` : `Zapnout ${r.name}`}
      />
    </span>
  )

  return (
    <>
      <PageHeader
        title="Pravidelné faktury"
        description="Faktury, které se vystavují samy podle šablony — měsíčně, čtvrtletně nebo ročně."
        actions={
          canEdit
            ? [{ label: 'Nová pravidelná faktura', icon: PlusIcon, primary: true, render: <Link to="/a/$slug/recurring/new" params={{ slug }} /> }]
            : undefined
        }
      >
        <RecurringTabs slug={slug} />
      </PageHeader>
      <PageBody>
        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <ResponsiveList
            items={items}
            isLoading={list.isPending}
            getKey={(r) => r.id}
            onRowClick={open}
            empty={
              <EmptyState
                icon={RepeatIcon}
                title="Žádné pravidelné faktury"
                description="Nastavte si opakované vystavení faktury ze šablony — třeba měsíční paušál nebo roční licenci."
                action={
                  canEdit && (
                    <ButtonLink to="/a/$slug/recurring/new" params={{ slug }}>
                      <PlusIcon data-icon="inline-start" />
                      Nová pravidelná faktura
                    </ButtonLink>
                  )
                }
              />
            }
            renderCard={(r) => {
              const i = info(r)
              return (
                <div className={cn('flex flex-col gap-2', !r.active && 'opacity-70')}>
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 flex-col">
                      <span className="flex min-w-0 items-center gap-1.5 font-medium">
                        <span className="truncate">{r.name}</span>
                        {r.last_error && <ErrorBadge error={r.last_error} />}
                      </span>
                      <span className="truncate text-sm text-muted-foreground">{i.client}</span>
                    </div>
                    {activeSwitch(r)}
                  </div>
                  <div className="flex items-center justify-between gap-3 text-sm">
                    <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
                      <RepeatIcon className="size-3.5 shrink-0" />
                      <span className="truncate">
                        {periodLabel(r.months_period)}
                        {r.active && ` · ${nextText(r)}`}
                        {!r.active && ' · vypnuto'}
                      </span>
                    </span>
                    <span className="shrink-0 font-semibold tabular-nums">{i.amount}</span>
                  </div>
                </div>
              )
            }}
            columns={[
              {
                id: 'name',
                header: 'Název',
                cell: (r) => {
                  const i = info(r)
                  return (
                    <span className={cn('flex min-w-0 flex-col', !r.active && 'opacity-70')}>
                      <span className="flex items-center gap-1.5 font-medium">
                        {r.name}
                        {r.last_error && <ErrorBadge error={r.last_error} />}
                        {r.send_email && <MailIcon className="size-3.5 text-muted-foreground" aria-label="Posílá se e-mailem" />}
                      </span>
                      <span className="text-xs text-muted-foreground">{i.client}</span>
                    </span>
                  )
                },
              },
              { id: 'period', header: 'Opakování', cell: (r) => <span className="text-muted-foreground">{periodLabel(r.months_period)}</span> },
              {
                id: 'next',
                header: 'Příští vystavení',
                cell: (r) =>
                  r.active ? (
                    <span className="flex items-center gap-1.5 whitespace-nowrap">
                      <CalendarClockIcon className="size-3.5 text-muted-foreground" />
                      {formatDate(r.next_occurrence_on)}
                    </span>
                  ) : (
                    <Badge variant="outline" className="text-muted-foreground">
                      Vypnuto
                    </Badge>
                  ),
              },
              { id: 'amount', header: 'Částka', align: 'right', cell: (r) => <span className="font-medium">{info(r).amount}</span> },
              { id: 'active', header: 'Aktivní', align: 'right', className: 'w-20', cell: activeSwitch },
            ]}
          />
        )}
      </PageBody>
    </>
  )
}

function amountText(t: InvoiceTemplate | undefined, account: Account | undefined): string {
  if (!t || !account) return ''
  return formatMoney(templateTotal(t, account), t.currency || account.default_currency)
}

function nextText(r: Recurring): string {
  if (r.end_on && r.next_occurrence_on > r.end_on) return 'ukončeno'
  if (r.next_occurrence_on < todayISO()) return 'čeká na vystavení'
  return `další ${formatDueRelative(r.next_occurrence_on)} (${formatDate(r.next_occurrence_on)})`
}

function ErrorBadge({ error }: { error: string }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Badge variant="destructive" className="shrink-0 gap-1">
            <TriangleAlertIcon data-icon="inline-start" />
            Chyba
          </Badge>
        }
      />
      <TooltipContent className="max-w-72">Poslední vystavení selhalo: {error}</TooltipContent>
    </Tooltip>
  )
}
