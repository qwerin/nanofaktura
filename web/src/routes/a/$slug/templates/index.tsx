import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { FilePlus2Icon, FileStackIcon, RepeatIcon } from 'lucide-react'
import { accountQueries } from '@/api/queries/accounts'
import { recurringQueries } from '@/api/queries/recurring'
import { templateQueries } from '@/api/queries/templates'
import type { InvoiceTemplate } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { documentTypeShortLabels } from '@/components/invoice/status'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { RecurringTabs } from '@/components/recurring/recurring-tabs'
import { templateTotal } from '@/components/recurring/template-model'
import { useSubjectMap } from '@/components/recurring/use-subject-map'
import { ResponsiveList } from '@/components/responsive-list'
import { Badge } from '@/components/ui/badge'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatMoney } from '@/lib/money'

export const Route = createFileRoute('/a/$slug/templates/')({
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(templateQueries.list(params.slug)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Šablony faktur · NanoFaktura' }] }),
  component: TemplatesPage,
})

function TemplatesPage() {
  const { slug } = Route.useParams()
  const { canEdit } = useCurrentAccount()
  const navigate = useNavigate()
  const list = useQuery(templateQueries.list(slug))
  const account = useQuery(accountQueries.detail(slug))
  const recurring = useQuery(recurringQueries.list(slug))
  const items = list.data?.items
  const subjects = useSubjectMap(slug, (items ?? []).map((t) => t.subject_id))

  const usage = new Map<number, number>()
  for (const r of recurring.data?.items ?? []) usage.set(r.template_id, (usage.get(r.template_id) ?? 0) + 1)

  const total = (t: InvoiceTemplate) => (account.data ? formatMoney(templateTotal(t, account.data), t.currency || account.data.default_currency) : '')
  const clientName = (t: InvoiceTemplate) => subjects.get(t.subject_id)?.name ?? '…'
  const open = (t: InvoiceTemplate) => navigate({ to: '/a/$slug/templates/$templateId', params: { slug, templateId: t.id } })

  return (
    <>
      <PageHeader
        title="Šablony faktur"
        description="Předvyplněné faktury pro opakované vystavování — ručně i automaticky."
        back={{ to: '/a/$slug/recurring', params: { slug } }}
        actions={
          canEdit
            ? [{ label: 'Nová šablona', icon: FilePlus2Icon, primary: true, render: <Link to="/a/$slug/templates/new" params={{ slug }} /> }]
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
            getKey={(t) => t.id}
            onRowClick={open}
            empty={
              <EmptyState
                icon={FileStackIcon}
                title="Zatím žádné šablony"
                description="Šablonu vytvoříte tady, nebo z hotové faktury akcí „Uložit jako šablonu“."
                action={
                  canEdit && (
                    <ButtonLink to="/a/$slug/templates/new" params={{ slug }}>
                      <FilePlus2Icon data-icon="inline-start" />
                      Nová šablona
                    </ButtonLink>
                  )
                }
              />
            }
            renderCard={(t) => (
              <div className="flex flex-col gap-1">
                <div className="flex items-baseline justify-between gap-3">
                  <span className="min-w-0 truncate font-medium">{t.name}</span>
                  <span className="shrink-0 font-semibold tabular-nums">{total(t)}</span>
                </div>
                <div className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
                  <span className="min-w-0 truncate">{clientName(t)}</span>
                  <TemplateBadges t={t} uses={usage.get(t.id) ?? 0} />
                </div>
              </div>
            )}
            columns={[
              { id: 'name', header: 'Šablona', cell: (t) => <span className="font-medium">{t.name}</span> },
              { id: 'client', header: 'Odběratel', cell: (t) => <span className="text-muted-foreground">{clientName(t)}</span> },
              { id: 'badges', header: '', cell: (t) => <TemplateBadges t={t} uses={usage.get(t.id) ?? 0} /> },
              { id: 'lines', header: 'Položek', align: 'right', cell: (t) => t.lines.length },
              { id: 'total', header: 'Částka', align: 'right', cell: (t) => <span className="font-medium">{total(t)}</span> },
            ]}
          />
        )}
      </PageBody>
    </>
  )
}

function TemplateBadges({ t, uses }: { t: InvoiceTemplate; uses: number }) {
  return (
    <span className="flex shrink-0 items-center gap-1">
      {t.document_type === 'proforma' && <Badge variant="outline">{documentTypeShortLabels.proforma}</Badge>}
      {uses > 0 && (
        <Badge variant="secondary" className="gap-1">
          <RepeatIcon data-icon="inline-start" />
          {uses > 1 ? `${uses}×` : 'opakuje se'}
        </Badge>
      )}
    </span>
  )
}
