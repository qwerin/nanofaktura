import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  CalendarClockIcon,
  FileTextIcon,
  MailIcon,
  PauseIcon,
  PencilIcon,
  PlayIcon,
  RepeatIcon,
  SendIcon,
  Trash2Icon,
  TriangleAlertIcon,
  ZapIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { invoiceQueries } from '@/api/queries/invoices'
import { recurringQueries, useDeleteRecurring, useRunRecurringNow, useSetRecurringActive } from '@/api/queries/recurring'
import { subjectQueries } from '@/api/queries/subjects'
import { templateQueries } from '@/api/queries/templates'
import type { Recurring } from '@/api/types'
import { StatusBadge } from '@/components/invoice/status-badge'
import { documentTypeLabels } from '@/components/invoice/status'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormCard } from '@/components/recurring/form-card'
import { periodLabel, renderDatePlaceholders } from '@/components/recurring/period'
import { templateTotal } from '@/components/recurring/template-model'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate, formatDateLong, formatDateTime, formatDueRelative, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { parseId } from '@/lib/params'
import { formatQuantity } from '@/lib/quantity'

export const Route = createFileRoute('/a/$slug/recurring/$recurringId/')({
  params: {
    parse: ({ recurringId }) => ({ recurringId: parseId(recurringId) }),
    stringify: ({ recurringId }) => ({ recurringId: String(recurringId) }),
  },
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(recurringQueries.detail(params.slug, params.recurringId)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Pravidelná faktura · NanoFaktura' }] }),
  component: RecurringDetailPage,
})

function RecurringDetailPage() {
  const { slug, recurringId } = Route.useParams()
  const recurring = useQuery(recurringQueries.detail(slug, recurringId))
  const back = { to: '/a/$slug/recurring', params: { slug } } as const

  if (recurring.isError) {
    return (
      <>
        <PageHeader title="Pravidelná faktura" back={back} />
        <PageError error={recurring.error} reset={() => void recurring.refetch()} />
      </>
    )
  }
  if (!recurring.data) {
    return (
      <>
        <PageHeader title="Pravidelná faktura" back={back} />
        <PageBody>
          <div className="flex flex-col gap-4" aria-busy="true" aria-label="Načítání">
            <Skeleton className="h-28 w-full rounded-xl" />
            <Skeleton className="h-64 w-full rounded-xl" />
          </div>
        </PageBody>
      </>
    )
  }
  return <RecurringDetail slug={slug} r={recurring.data} />
}

type ConfirmKind = 'run' | 'delete' | null

function RecurringDetail({ slug, r }: { slug: string; r: Recurring }) {
  const navigate = useNavigate()
  const { canEdit } = useCurrentAccount()
  const account = useQuery(accountQueries.detail(slug))
  const template = useQuery({ ...templateQueries.detail(slug, r.template_id), retry: false })
  const subject = useQuery({ ...subjectQueries.detail(slug, template.data?.subject_id ?? 0), enabled: Boolean(template.data), retry: false })
  const lastInvoice = useQuery({ ...invoiceQueries.detail(slug, r.last_invoice_id ?? 0), enabled: Boolean(r.last_invoice_id), retry: false })
  const setActive = useSetRecurringActive(slug)
  const runNow = useRunRecurringNow(slug, r.id)
  const remove = useDeleteRecurring(slug)
  const [confirm, setConfirm] = useState<ConfirmKind>(null)

  const ended = Boolean(r.end_on && r.next_occurrence_on > r.end_on)
  const today = todayISO()
  const lang = template.data?.language === 'en' ? 'en' : 'cs'
  const currency = template.data?.currency || account.data?.default_currency || 'CZK'
  const busy = setActive.isPending || runNow.isPending || remove.isPending

  const actions: PageAction[] = canEdit
    ? [
        { label: 'Vystavit teď', icon: ZapIcon, primary: true, variant: 'default', onClick: () => setConfirm('run'), disabled: busy },
        { label: 'Upravit', icon: PencilIcon, render: <Link to="/a/$slug/recurring/$recurringId/edit" params={{ slug, recurringId: r.id }} /> },
        r.active
          ? {
              label: 'Vypnout',
              icon: PauseIcon,
              overflow: true,
              disabled: busy,
              onClick: () => setActive.mutate({ id: r.id, active: false }, { onSuccess: () => toast.success('Pravidelná faktura vypnuta') }),
            }
          : {
              label: 'Zapnout',
              icon: PlayIcon,
              overflow: true,
              disabled: busy || ended,
              onClick: () =>
                setActive.mutate(
                  { id: r.id, active: true },
                  { onSuccess: (res) => toast.success(`Zapnuto · příští vystavení ${formatDate(res.next_occurrence_on)}`) },
                ),
            },
        { label: 'Smazat', icon: Trash2Icon, overflow: true, variant: 'destructive', onClick: () => setConfirm('delete'), disabled: busy },
      ]
    : []

  return (
    <>
      <PageHeader
        title={
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate">{r.name}</span>
            {busy && <Spinner className="size-4 text-muted-foreground" />}
          </span>
        }
        description="Pravidelná faktura"
        back={{ to: '/a/$slug/recurring', params: { slug } }}
        actions={actions}
      />
      <PageBody>
        {/* Souhrn */}
        <section className="mb-4 flex flex-col gap-4 rounded-xl border bg-card p-4 md:flex-row md:items-center md:justify-between md:p-5">
          <div className="flex min-w-0 flex-col gap-1.5">
            <div className="flex flex-wrap items-center gap-2">
              {r.active ? (
                <Badge className="gap-1 bg-success/15 text-success dark:bg-success/20">
                  <RepeatIcon data-icon="inline-start" />
                  Aktivní
                </Badge>
              ) : (
                <Badge variant="outline" className="text-muted-foreground">
                  {ended ? 'Ukončeno' : 'Vypnuto'}
                </Badge>
              )}
              <Badge variant="outline">{documentTypeLabels[r.issue_as]}</Badge>
              {r.send_email && (
                <Badge variant="secondary" className="gap-1">
                  <MailIcon data-icon="inline-start" />
                  E-mailem
                </Badge>
              )}
            </div>
            <p className="truncate text-lg font-semibold tracking-tight md:text-xl">{subject.data?.name ?? (template.isError ? 'Šablona chybí' : '…')}</p>
            <p className="text-sm text-muted-foreground first-letter:uppercase">
              {periodLabel(r.months_period)}
              {r.day_of_month ? ` · ${r.day_of_month === 31 ? 'poslední den měsíce' : `${r.day_of_month}. den v měsíci`}` : ''} · od{' '}
              {formatDate(r.start_on)}
              {r.end_on ? ` do ${formatDate(r.end_on)}` : ''}
            </p>
          </div>
          <div className="flex items-end justify-between gap-6 md:flex-col md:items-end md:gap-1">
            <div className="md:text-right">
              <p className="text-xs text-muted-foreground">Příští vystavení</p>
              <p className="text-lg font-semibold tracking-tight">
                {r.active && !ended ? formatDateLong(r.next_occurrence_on) : '—'}
              </p>
              {r.active && !ended && r.next_occurrence_on >= today && (
                <p className="text-xs text-muted-foreground">{formatDueRelative(r.next_occurrence_on, today)}</p>
              )}
            </div>
            {template.data && account.data && (
              <div className="md:text-right">
                <p className="text-xs text-muted-foreground">Částka</p>
                <p className="text-lg font-semibold tabular-nums">{formatMoney(templateTotal(template.data, account.data), currency)}</p>
              </div>
            )}
          </div>
        </section>

        {r.last_error && (
          <Alert variant="destructive" className="mb-4">
            <TriangleAlertIcon />
            <AlertTitle>Poslední vystavení selhalo</AlertTitle>
            <AlertDescription>
              <span className="break-words">{r.last_error}</span>
              <span>Plánovač to zkusí znovu při dalším běhu (každou hodinu). Opravte příčinu, případně vystavte ručně tlačítkem „Vystavit teď“.</span>
            </AlertDescription>
          </Alert>
        )}

        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start lg:gap-6">
          <FormCard
            title="Šablona"
            description={template.data?.name}
            action={
              template.data && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="max-md:h-9"
                  nativeButton={false}
                  render={<Link to="/a/$slug/templates/$templateId" params={{ slug, templateId: r.template_id }} />}
                >
                  <PencilIcon data-icon="inline-start" />
                  Upravit šablonu
                </Button>
              )
            }
          >
            {template.isError ? (
              <p className="text-sm text-destructive">Šablona se nepodařila načíst.</p>
            ) : !template.data ? (
              <Skeleton className="h-24 w-full" />
            ) : (
              <>
                <p className="text-sm text-muted-foreground">
                  Položky tak, jak budou na faktuře k {formatDate(r.active && !ended ? r.next_occurrence_on : today)}:
                </p>
                <ul className="-my-1 divide-y">
                  {template.data.lines.map((l, i) => (
                    <li key={i} className="flex flex-col gap-0.5 py-2.5">
                      <div className="flex items-baseline justify-between gap-3">
                        <span className="min-w-0 font-medium">
                          {renderDatePlaceholders(l.name, r.active && !ended ? r.next_occurrence_on : today, lang)}
                        </span>
                        <span className="shrink-0 tabular-nums">{formatMoney(l.unit_price, currency)}</span>
                      </div>
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {formatQuantity(l.quantity)} {l.unit_name}
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </FormCard>

          <FormCard title="Historie">
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
              <Info label="Poslední běh">{r.last_run_at ? formatDateTime(r.last_run_at) : 'zatím neproběhl'}</Info>
              <Info label="Poslední faktura">
                {r.last_invoice_id ? (
                  <Link
                    to="/a/$slug/invoices/$invoiceId"
                    params={{ slug, invoiceId: r.last_invoice_id }}
                    className="inline-flex items-center gap-1.5 font-medium text-primary underline-offset-4 hover:underline"
                  >
                    <FileTextIcon className="size-3.5" />
                    {lastInvoice.data?.number ?? 'Zobrazit'}
                  </Link>
                ) : (
                  '—'
                )}
              </Info>
              {lastInvoice.data && (
                <>
                  <Info label="Stav">
                    <StatusBadge status={lastInvoice.data.status} />
                  </Info>
                  <Info label="Vystavena">{formatDate(lastInvoice.data.issued_on)}</Info>
                </>
              )}
              <Info label="Založeno">{formatDateTime(r.created_at)}</Info>
            </dl>
            {r.send_email && (
              <p className="flex items-start gap-2 rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
                <SendIcon className="mt-0.5 size-3.5 shrink-0" />
                Faktury se posílají na e-mail odběratele{subject.data && !subject.data.email ? ' — ten ale nemá vyplněný e-mail!' : '.'}
              </p>
            )}
            <p className="flex items-start gap-2 text-xs text-muted-foreground">
              <CalendarClockIcon className="mt-0.5 size-3.5 shrink-0" />
              Plánovač kontroluje pravidelné faktury každou hodinu.
            </p>
          </FormCard>
        </div>
      </PageBody>

      <ResponsiveDialog
        open={confirm === 'run'}
        onOpenChange={(o) => !runNow.isPending && setConfirm(o ? 'run' : null)}
        title="Vystavit fakturu teď?"
        description={
          <>
            Vystaví se {r.issue_as === 'proforma' ? 'zálohová faktura' : 'faktura'} s dnešním datem
            {r.send_email ? ' a odešle se klientovi e-mailem' : ''}. Příští pravidelné vystavení se posune o jednu periodu.
          </>
        }
        footer={
          <>
            <Button
              disabled={runNow.isPending}
              onClick={() =>
                runNow.mutate(undefined, {
                  onSuccess: (inv) => {
                    toast.success(`${documentTypeLabels[inv.document_type]} ${inv.number} vystavena`)
                    setConfirm(null)
                    void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: inv.id } })
                  },
                })
              }
            >
              {runNow.isPending && <Spinner data-icon="inline-start" />}
              Vystavit
            </Button>
            <Button variant="outline" onClick={() => setConfirm(null)} disabled={runNow.isPending}>
              Zpět
            </Button>
          </>
        }
      />
      <ResponsiveDialog
        open={confirm === 'delete'}
        onOpenChange={(o) => !remove.isPending && setConfirm(o ? 'delete' : null)}
        title={`Smazat „${r.name}“?`}
        description="Další faktury se už nebudou vystavovat. Vystavené faktury i šablona zůstanou."
        footer={
          <>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(r.id, {
                  onSuccess: () => {
                    toast.success('Pravidelná faktura smazána')
                    void navigate({ to: '/a/$slug/recurring', params: { slug }, replace: true })
                  },
                })
              }
            >
              {remove.isPending && <Spinner data-icon="inline-start" />}
              Smazat
            </Button>
            <Button variant="outline" onClick={() => setConfirm(null)} disabled={remove.isPending}>
              Zpět
            </Button>
          </>
        }
      />
    </>
  )
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-right">{children}</dd>
    </>
  )
}
