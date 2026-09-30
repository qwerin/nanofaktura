import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import {
  EllipsisVerticalIcon,
  HistoryIcon,
  KeyRoundIcon,
  PencilIcon,
  PlusIcon,
  PowerIcon,
  PowerOffIcon,
  SendIcon,
  Trash2Icon,
  TriangleAlertIcon,
  WebhookIcon,
} from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { errorMessage } from '@/api/errors'
import { useTestWebhook, useUpdateWebhook, webhookQueries } from '@/api/queries/webhooks'
import type { Webhook, WebhookTestResult } from '@/api/types'
import { useNow } from '@/components/bank/use-now'
import { EmptyState } from '@/components/empty-state'
import { PageError } from '@/components/page-states'
import { SettingsPage } from '@/components/settings-page'
import { ListSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import { DeliveriesDialog } from '@/components/webhooks/deliveries-dialog'
import { SignatureDocs } from '@/components/webhooks/signature-docs'
import { DeleteWebhookDialog, SecretDialog, TestResultDialog, WebhookFormDialog } from '@/components/webhooks/webhook-dialogs'
import { describeEvents, httpStatusTone } from '@/components/webhooks/webhook-events'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDateTime } from '@/lib/date'
import { formatRelativeTime } from '@/lib/relative-time'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/settings/webhooks')({
  head: () => ({ meta: [{ title: 'Webhooky · NanoFaktura' }] }),
  component: WebhooksPage,
})

function WebhooksPage() {
  const { slug } = Route.useParams()
  const { canManageSettings } = useCurrentAccount()
  const list = useQuery({ ...webhookQueries.list(slug), enabled: canManageSettings })
  const [form, setForm] = useState<{ webhook: Webhook | null } | null>(null)
  const [secretOf, setSecretOf] = useState<Webhook | null>(null)
  const [deliveriesOf, setDeliveriesOf] = useState<Webhook | null>(null)
  const [deleting, setDeleting] = useState<Webhook | null>(null)
  const [testResult, setTestResult] = useState<WebhookTestResult | null>(null)
  const now = useNow()

  const openCreate = () => setForm({ webhook: null })

  return (
    <SettingsPage
      title="Webhooky"
      description="Webhook pošle při vybraných událostech (nová faktura, platba, e-mail …) POST s JSONem na vaši adresu — pro napojení CRM, účetnictví nebo automatizací. Požadavky jsou podepsané tajemstvím webhooku."
      actions={canManageSettings ? [{ label: 'Nový webhook', icon: PlusIcon, primary: true, onClick: openCreate }] : undefined}
    >
      {!canManageSettings ? (
        <Alert>
          <TriangleAlertIcon />
          <AlertDescription>Webhooky mohou zobrazit a spravovat jen vlastník a administrátoři účtu.</AlertDescription>
        </Alert>
      ) : list.isError ? (
        <PageError error={list.error} reset={() => void list.refetch()} />
      ) : !list.data ? (
        <ListSkeleton />
      ) : (
        <div className="flex flex-col gap-4">
          {list.data.length > 0 && (
            <div className="hidden justify-end md:flex">
              <Button onClick={openCreate}>
                <PlusIcon data-icon="inline-start" />
                Nový webhook
              </Button>
            </div>
          )}
          {list.data.length === 0 ? (
            <EmptyState
              icon={WebhookIcon}
              title="Žádné webhooky"
              description="Přidejte adresu, na kterou máme posílat oznámení o událostech v účtu."
              action={
                <Button onClick={openCreate}>
                  <PlusIcon data-icon="inline-start" />
                  Nový webhook
                </Button>
              }
            />
          ) : (
            <ul className="flex flex-col gap-3">
              {list.data.map((w) => (
                <WebhookCard
                  key={w.id}
                  slug={slug}
                  webhook={w}
                  now={now}
                  onEdit={() => setForm({ webhook: w })}
                  onDeliveries={() => setDeliveriesOf(w)}
                  onDelete={() => setDeleting(w)}
                  onSecret={setSecretOf}
                  onTested={setTestResult}
                />
              ))}
            </ul>
          )}
          <SignatureDocs />
        </div>
      )}

      <WebhookFormDialog
        slug={slug}
        open={form !== null}
        webhook={form?.webhook ?? null}
        onClose={() => setForm(null)}
        onCreated={(w) => {
          setForm(null)
          setSecretOf(w)
        }}
      />
      <SecretDialog webhook={secretOf} onClose={() => setSecretOf(null)} />
      <TestResultDialog result={testResult} onClose={() => setTestResult(null)} />
      <DeliveriesDialog slug={slug} webhook={deliveriesOf} onClose={() => setDeliveriesOf(null)} />
      <DeleteWebhookDialog slug={slug} webhook={deleting} onClose={() => setDeleting(null)} />
    </SettingsPage>
  )
}

function WebhookCard({
  slug,
  webhook: w,
  now,
  onEdit,
  onDeliveries,
  onDelete,
  onSecret,
  onTested,
}: {
  slug: string
  webhook: Webhook
  now: number
  onEdit: () => void
  onDeliveries: () => void
  onDelete: () => void
  onSecret: (w: Webhook) => void
  onTested: (r: WebhookTestResult) => void
}) {
  const test = useTestWebhook(slug)
  const update = useUpdateWebhook(slug)
  const events = describeEvents(w.events)
  const tone = httpStatusTone(w.last_status)

  const rotate = () =>
    update.mutate(
      { id: w.id, body: { rotate_secret: true } },
      { onSuccess: (updated) => onSecret(updated), onError: (err) => toast.error(errorMessage(err)) },
    )
  const setActive = (active: boolean) =>
    update.mutate(
      { id: w.id, body: { active } },
      {
        onSuccess: () => toast.success(active ? 'Webhook zapnut' : 'Webhook vypnut'),
        onError: (err) => toast.error(errorMessage(err)),
      },
    )

  return (
    <li className="flex flex-col gap-3 rounded-xl border bg-card p-4">
      <div className="flex items-start gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            {w.disabled_at ? (
              <Badge variant="destructive" title={`Vypnuto ${formatDateTime(w.disabled_at)} po opakovaných chybách`}>
                Vypnuto kvůli chybám
              </Badge>
            ) : w.active ? (
              <Badge variant="secondary" className="bg-success/12 text-success dark:bg-success/20">
                Aktivní
              </Badge>
            ) : (
              <Badge variant="secondary">Neaktivní</Badge>
            )}
            {w.consecutive_failures > 0 && !w.disabled_at && (
              <Badge variant="secondary" className="bg-warning/15 text-warning-foreground dark:text-warning">
                {w.consecutive_failures}× neúspěšně za sebou
              </Badge>
            )}
          </div>
          <p className="font-mono text-sm break-all">{w.url}</p>
          {w.description && <p className="text-sm text-muted-foreground">{w.description}</p>}
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="ghost" size="icon" className="-mt-1 -mr-2 shrink-0" aria-label="Další akce" />}>
            <EllipsisVerticalIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-52">
            <DropdownMenuItem onClick={onEdit}>
              <PencilIcon />
              Upravit
            </DropdownMenuItem>
            <DropdownMenuItem onClick={rotate} disabled={update.isPending}>
              <KeyRoundIcon />
              Vygenerovat nové tajemství
            </DropdownMenuItem>
            {w.active && !w.disabled_at ? (
              <DropdownMenuItem onClick={() => setActive(false)} disabled={update.isPending}>
                <PowerOffIcon />
                Vypnout
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={() => setActive(true)} disabled={update.isPending}>
                <PowerIcon />
                Zapnout
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={onDelete}>
              <Trash2Icon />
              Smazat
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <div className="flex flex-wrap gap-1.5">
        {events.slice(0, 5).map((e) => (
          <Badge key={e} variant="outline" className="font-normal">
            {e}
          </Badge>
        ))}
        {events.length > 5 && <Badge variant="outline">+{events.length - 5}</Badge>}
      </div>

      <div className="flex flex-col gap-3 border-t pt-3 sm:flex-row sm:items-center">
        <p className="flex-1 text-xs text-muted-foreground">
          {w.last_delivered_at ? (
            <>
              Poslední pokus{' '}
              <time dateTime={w.last_delivered_at} title={formatDateTime(w.last_delivered_at)}>
                {formatRelativeTime(w.last_delivered_at, new Date(now))}
              </time>
              {' · '}
              <span className={cn('font-medium', tone === 'success' ? 'text-success' : tone === 'destructive' ? 'text-destructive' : '')}>
                {w.last_status ? `HTTP ${w.last_status}` : 'bez odpovědi'}
              </span>
              {w.last_error && <span className="block truncate text-destructive">{w.last_error}</span>}
            </>
          ) : (
            'Zatím nic neodešlo.'
          )}
        </p>
        <div className="grid grid-cols-2 gap-2 sm:flex">
          <Button
            variant="outline"
            size="sm"
            className="max-md:h-10"
            disabled={test.isPending}
            onClick={() => test.mutate(w, { onSuccess: onTested })}
          >
            {test.isPending ? <Spinner data-icon="inline-start" /> : <SendIcon data-icon="inline-start" />}
            Poslat test
          </Button>
          <Button variant="outline" size="sm" className="max-md:h-10" onClick={onDeliveries}>
            <HistoryIcon data-icon="inline-start" />
            Doručení
          </Button>
        </div>
      </div>
    </li>
  )
}
