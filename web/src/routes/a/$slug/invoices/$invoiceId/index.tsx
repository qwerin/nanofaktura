import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  BanIcon,
  BanknoteIcon,
  CopyIcon,
  DownloadIcon,
  FileMinusIcon,
  FileTextIcon,
  LinkIcon,
  LockIcon,
  LockOpenIcon,
  PencilIcon,
  RotateCcwIcon,
  SendIcon,
  Share2Icon,
  Trash2Icon,
  CircleSlashIcon,

} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import {
  invoicePdfUrl,
  invoiceQueries,
  useCreateCorrection,
  useDeleteInvoice,
  useDeletePayment,
  useDuplicateInvoice,
  useInvoiceAction,
} from '@/api/queries/invoices'
import type { Invoice, InvoiceAction, InvoicePayment } from '@/api/types'
import { AddPaymentDialog } from '@/components/invoice/add-payment-dialog'
import { DueText } from '@/components/invoice/due-text'
import { StatusBadge } from '@/components/invoice/status-badge'
import { allowedActions, documentTypeLabels, paymentMethodLabels, type UiAction } from '@/components/invoice/status'
import { TotalsPanel } from '@/components/invoice/totals-panel'
import { useCanEditDocuments } from '@/components/invoice/use-can-edit'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { useIsMobile } from '@/hooks/use-mobile'
import { formatDate, formatDateTime } from '@/lib/date'
import { formatMoney, formatVatRate } from '@/lib/money'
import { parseId } from '@/lib/params'
import { formatQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/invoices/$invoiceId/')({
  // ID z URL jako číslo; neplatné ID → 404.
  params: {
    parse: ({ invoiceId }) => ({ invoiceId: parseId(invoiceId) }),
    stringify: ({ invoiceId }) => ({ invoiceId: String(invoiceId) }),
  },
  loader: ({ context, params }) =>
    context.queryClient.prefetchQuery(invoiceQueries.detail(params.slug, params.invoiceId)),
  head: () => ({ meta: [{ title: 'Faktura · NanoFaktura' }] }),
  component: InvoiceDetailPage,
})

type Confirm = {
  title: string
  description: ReactNode
  confirmLabel: string
  destructive?: boolean
  run: () => Promise<unknown>
}

function InvoiceDetailPage() {
  const { slug, invoiceId } = Route.useParams()
  const invoice = useQuery(invoiceQueries.detail(slug, invoiceId))
  const back = { to: '/a/$slug/invoices', params: { slug } } as const

  if (invoice.isError) {
    return (
      <>
        <PageHeader title="Faktura" back={back} />
        <PageError error={invoice.error} reset={() => void invoice.refetch()} />
      </>
    )
  }
  if (!invoice.data) {
    return (
      <>
        <PageHeader title="Faktura" back={back} />
        <PageBody>
          <DetailSkeleton />
        </PageBody>
      </>
    )
  }
  return <InvoiceDetail slug={slug} inv={invoice.data} />
}

function InvoiceDetail({ slug, inv }: { slug: string; inv: Invoice }) {
  const navigate = useNavigate()
  const isMobile = useIsMobile()
  const canEdit = useCanEditDocuments()
  const [paymentOpen, setPaymentOpen] = useState(false)
  const [confirm, setConfirm] = useState<Confirm | null>(null)
  const [confirmPending, setConfirmPending] = useState(false)

  const action = useInvoiceAction(slug, inv.id)
  const duplicate = useDuplicateInvoice(slug, inv.id)
  const correction = useCreateCorrection(slug, inv.id)
  const remove = useDeleteInvoice(slug)
  const deletePayment = useDeletePayment(slug, inv.id)

  const allowed = canEdit ? allowedActions({ ...inv, payment_count: inv.payments.length }) : new Set<UiAction>()
  const payer = inv.your_vat_mode !== 'non_vat_payer'
  const pdfUrl = invoicePdfUrl(slug, inv.id)
  const pdfName = `faktura-${inv.number.replace(/[^A-Za-z0-9._-]/g, '-')}.pdf`
  const publicUrl = inv.public_token ? `${window.location.origin}/p/${inv.public_token}` : null

  const run = (a: InvoiceAction, msg: string) =>
    action.mutate(a, { onSuccess: () => toast.success(msg) })

  const ask = (c: Confirm) => setConfirm(c)

  const sharePublic = async () => {
    if (!publicUrl) return
    if (isMobile && typeof navigator.share === 'function') {
      try {
        await navigator.share({ title: `${documentTypeLabels[inv.document_type]} ${inv.number}`, url: publicUrl })
      } catch {
        /* zrušeno uživatelem */
      }
      return
    }
    try {
      await navigator.clipboard.writeText(publicUrl)
      toast.success('Odkaz pro klienta zkopírován')
    } catch {
      toast.error('Kopírování se nezdařilo.')
    }
  }

  const act = (key: UiAction, a: Omit<PageAction, 'label'> & { label: string }): PageAction[] => (allowed.has(key) ? [a] : [])

  const actions: PageAction[] = [
    ...act('edit', {
      label: 'Upravit',
      icon: PencilIcon,
      render: <Link to="/a/$slug/invoices/$invoiceId/edit" params={{ slug, invoiceId: inv.id }} />,
    }),
    {
      label: isMobile ? 'PDF' : 'Otevřít PDF',
      icon: FileTextIcon,
      primary: true,
      variant: 'outline',
      render: <a href={pdfUrl} target="_blank" rel="noopener" />,
    },
    ...act('add_payment', {
      label: 'Přidat platbu',
      icon: BanknoteIcon,
      primary: true,
      variant: 'default',
      onClick: () => setPaymentOpen(true),
    }),
    ...(isMobile
      ? []
      : [{ label: 'Stáhnout PDF', icon: DownloadIcon, overflow: true, render: <a href={pdfUrl} download={pdfName} /> }]),
    ...act('mark_as_sent', {
      label: 'Označit jako odeslanou',
      icon: SendIcon,
      overflow: true,
      onClick: () => run('mark_as_sent', 'Označeno jako odeslané'),
    }),
    ...(publicUrl
      ? [
          {
            label: isMobile ? 'Sdílet odkaz pro klienta' : 'Kopírovat odkaz pro klienta',
            icon: isMobile ? Share2Icon : LinkIcon,
            overflow: true,
            onClick: () => void sharePublic(),
          },
        ]
      : []),
    ...(canEdit
      ? [
          {
            label: 'Duplikovat',
            icon: CopyIcon,
            overflow: true,
            onClick: () =>
              duplicate.mutate(undefined, {
                onSuccess: (d) => {
                  toast.success(`Vytvořena kopie ${d.number}`)
                  void navigate({ to: '/a/$slug/invoices/$invoiceId/edit', params: { slug, invoiceId: d.id } })
                },
              }),
          },
        ]
      : []),
    ...act('correction', {
      label: 'Vystavit opravný doklad',
      icon: FileMinusIcon,
      overflow: true,
      onClick: () =>
        ask({
          title: 'Vystavit opravný doklad?',
          description: `Vytvoří se opravný daňový doklad k ${inv.number} se zápornými množstvími. Pak ho můžete upravit.`,
          confirmLabel: 'Vystavit',
          run: () =>
            correction.mutateAsync().then((c) => {
              toast.success(`Opravný doklad ${c.number} vystaven`)
              void navigate({ to: '/a/$slug/invoices/$invoiceId/edit', params: { slug, invoiceId: c.id } })
            }),
        }),
    }),
    ...act('lock', { label: 'Zamknout', icon: LockIcon, overflow: true, onClick: () => run('lock', 'Doklad zamčen') }),
    ...act('unlock', { label: 'Odemknout', icon: LockOpenIcon, overflow: true, onClick: () => run('unlock', 'Doklad odemčen') }),
    ...act('cancel', {
      label: 'Stornovat',
      icon: BanIcon,
      overflow: true,
      onClick: () =>
        ask({
          title: 'Stornovat doklad?',
          description: 'Doklad zůstane v evidenci jako stornovaný a nebude se počítat do tržeb. Storno jde vrátit.',
          confirmLabel: 'Stornovat',
          destructive: true,
          run: () => action.mutateAsync('cancel').then(() => toast.success('Doklad stornován')),
        }),
    }),
    ...act('undo_cancel', {
      label: 'Obnovit (zrušit storno)',
      icon: RotateCcwIcon,
      overflow: true,
      onClick: () => run('undo_cancel', 'Storno zrušeno'),
    }),
    ...act('mark_as_uncollectible', {
      label: 'Označit jako nedobytnou',
      icon: CircleSlashIcon,
      overflow: true,
      onClick: () =>
        ask({
          title: 'Označit jako nedobytnou?',
          description: 'Pohledávka se přestane zobrazovat mezi neuhrazenými. Jde to vrátit.',
          confirmLabel: 'Označit',
          destructive: true,
          run: () => action.mutateAsync('mark_as_uncollectible').then(() => toast.success('Označeno jako nedobytné')),
        }),
    }),
    ...act('undo_uncollectible', {
      label: 'Obnovit pohledávku',
      icon: RotateCcwIcon,
      overflow: true,
      onClick: () => run('undo_uncollectible', 'Pohledávka obnovena'),
    }),
    ...act('delete', {
      label: 'Smazat',
      icon: Trash2Icon,
      overflow: true,
      variant: 'destructive',
      onClick: () =>
        ask({
          title: `Smazat ${inv.number}?`,
          description: 'Doklad se trvale odstraní. Pokud už byl odeslán klientovi, raději ho stornujte.',
          confirmLabel: 'Smazat',
          destructive: true,
          run: () =>
            remove.mutateAsync(inv.id).then(() => {
              toast.success('Doklad smazán')
              void navigate({ to: '/a/$slug/invoices', params: { slug }, replace: true })
            }),
        }),
    }),
  ]

  const title = inv.number
  const client = [inv.client_street, [inv.client_zip, inv.client_city].filter(Boolean).join(' ')].filter(Boolean)
  const busy = action.isPending || duplicate.isPending || correction.isPending

  return (
    <>
      <PageHeader
        title={
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate">{title}</span>
            {busy && <Spinner className="size-4 text-muted-foreground" />}
          </span>
        }
        description={documentTypeLabels[inv.document_type]}
        back={{ to: '/a/$slug/invoices', params: { slug } }}
        actions={actions}
      />
      <PageBody>
        {/* Souhrn */}
        <section className="mb-4 flex flex-col gap-4 rounded-xl border bg-card p-4 md:flex-row md:items-center md:justify-between md:p-5">
          <div className="flex min-w-0 flex-col gap-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge status={inv.status} />
              {inv.locked_at && (
                <Badge variant="outline" className="gap-1 text-muted-foreground">
                  <LockIcon data-icon="inline-start" />
                  Zamčeno
                </Badge>
              )}
              {inv.document_type !== 'invoice' && <Badge variant="outline">{documentTypeLabels[inv.document_type]}</Badge>}
            </div>
            <p className="truncate text-lg font-semibold tracking-tight md:text-xl">{inv.client_name}</p>
            <p className="text-sm text-muted-foreground">
              Vystaveno {formatDate(inv.issued_on)} · splatnost {formatDate(inv.due_on)}{' '}
              <DueText invoice={inv} className="whitespace-nowrap" />
            </p>
          </div>
          <div className="flex items-end justify-between gap-6 md:flex-col md:items-end md:gap-1">
            <div className="md:text-right">
              <p className="text-xs text-muted-foreground">Celkem</p>
              <p className="text-2xl font-semibold tracking-tight tabular-nums">{formatMoney(inv.total, inv.currency)}</p>
            </div>
            {inv.status !== 'cancelled' && inv.remaining_amount !== 0 && inv.remaining_amount !== inv.total && (
              <p className="text-sm text-muted-foreground md:text-right">
                zbývá <span className="font-medium text-foreground tabular-nums">{formatMoney(inv.remaining_amount, inv.currency)}</span>
              </p>
            )}
          </div>
        </section>

        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start lg:gap-6">
          <div className="flex min-w-0 flex-col gap-4">
            <Section title="Odběratel a dodavatel">
              <div className="grid gap-4 sm:grid-cols-2">
                <Party
                  label="Odběratel"
                  name={inv.client_name}
                  lines={[
                    inv.client_full_name,
                    ...client,
                    inv.client_country && inv.client_country !== 'CZ' ? inv.client_country : '',
                    inv.client_registration_no && `IČO ${inv.client_registration_no}`,
                    inv.client_vat_no && `DIČ ${inv.client_vat_no}`,
                    inv.client_email,
                  ]}
                />
                <Party
                  label="Dodavatel"
                  name={inv.your_name}
                  muted
                  lines={[
                    inv.your_street,
                    [inv.your_zip, inv.your_city].filter(Boolean).join(' '),
                    inv.your_registration_no && `IČO ${inv.your_registration_no}`,
                    inv.your_vat_no && `DIČ ${inv.your_vat_no}`,
                    inv.your_vat_mode === 'non_vat_payer' ? 'Neplátce DPH' : '',
                  ]}
                />
              </div>
            </Section>

            <Section title="Položky">
              <LinesView inv={inv} payer={payer} />
            </Section>

            {(inv.note || inv.footer_note || inv.private_note || inv.tags.length > 0) && (
              <Section title="Poznámky">
                <dl className="flex flex-col gap-3 text-sm">
                  {inv.note && <Note label="Text nad položkami">{inv.note}</Note>}
                  {inv.footer_note && <Note label="Patička">{inv.footer_note}</Note>}
                  {inv.private_note && <Note label="Soukromá poznámka">{inv.private_note}</Note>}
                  {inv.tags.length > 0 && (
                    <Note label="Štítky">
                      <span className="flex flex-wrap gap-1">
                        {inv.tags.map((t) => (
                          <Badge key={t} variant="secondary">
                            {t}
                          </Badge>
                        ))}
                      </span>
                    </Note>
                  )}
                </dl>
              </Section>
            )}
          </div>

          <div className="flex min-w-0 flex-col gap-4">
            <Section title={payer ? 'Rekapitulace DPH' : 'Součty'}>
              <TotalsPanel
                totals={{ vatRecap: inv.vat_recap.map((r) => ({ ...r, vatRateBps: r.vat_rate_bps })), subtotal: inv.subtotal, vatTotal: inv.vat_total, rounding: inv.rounding, total: inv.total }}
                currency={inv.currency}
                showVat={payer}
                reverseCharge={inv.reverse_charge}
                paid={inv.paid_amount}
                remaining={inv.remaining_amount}
              />
            </Section>

            <Section
              title="Platby"
              action={
                allowed.has('add_payment') && (
                  <Button variant="ghost" size="sm" onClick={() => setPaymentOpen(true)} className="max-md:h-9">
                    <BanknoteIcon data-icon="inline-start" />
                    Přidat
                  </Button>
                )
              }
            >
              {inv.payments.length === 0 ? (
                <p className="text-sm text-muted-foreground">Zatím žádná platba.</p>
              ) : (
                <ul className="-my-1 divide-y">
                  {inv.payments.map((p) => (
                    <PaymentRow
                      key={p.id}
                      p={p}
                      currency={inv.currency}
                      canDelete={canEdit}
                      onDelete={() =>
                        ask({
                          title: 'Smazat platbu?',
                          description: `Platba ${formatMoney(p.amount, inv.currency)} z ${formatDate(p.paid_on)} se odstraní a stav faktury se přepočítá.`,
                          confirmLabel: 'Smazat platbu',
                          destructive: true,
                          run: () => deletePayment.mutateAsync(p.id).then(() => toast.success('Platba smazána')),
                        })
                      }
                    />
                  ))}
                </ul>
              )}
            </Section>

            <Section title="Údaje">
              <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm">
                <Info label="Vystaveno">{formatDate(inv.issued_on)}</Info>
                {payer && inv.taxable_fulfillment_due && <Info label="DUZP">{formatDate(inv.taxable_fulfillment_due)}</Info>}
                <Info label="Splatnost">
                  {formatDate(inv.due_on)} <span className="text-muted-foreground">({inv.due_days} dní)</span>
                </Info>
                {inv.paid_on && <Info label="Uhrazeno">{formatDate(inv.paid_on)}</Info>}
                <Info label="Var. symbol">{inv.variable_symbol || '—'}</Info>
                <Info label="Úhrada">
                  {inv.payment_method === 'custom' ? inv.custom_payment_method || 'Jiný' : paymentMethodLabels[inv.payment_method]}
                </Info>
                {inv.payment_method === 'bank' && (inv.bank_account || inv.iban) && (
                  <Info label="Účet">
                    <span className="break-all">{inv.bank_account || inv.iban}</span>
                  </Info>
                )}
                {inv.currency !== 'CZK' && <Info label="Kurz">{inv.exchange_rate}</Info>}
                {inv.order_number && <Info label="Objednávka">{inv.order_number}</Info>}
                {inv.related_id && (
                  <Info label={inv.document_type === 'correction' ? 'Opravuje' : 'Související'}>
                    <Link
                      to="/a/$slug/invoices/$invoiceId"
                      params={{ slug, invoiceId: inv.related_id }}
                      className="text-primary underline-offset-4 hover:underline"
                    >
                      Zobrazit doklad
                    </Link>
                  </Info>
                )}
                {inv.sent_at && <Info label="Odesláno">{formatDateTime(inv.sent_at)}</Info>}
              </dl>
            </Section>
          </div>
        </div>
      </PageBody>

      {allowed.has('add_payment') && (
        <AddPaymentDialog slug={slug} invoice={inv} open={paymentOpen} onOpenChange={setPaymentOpen} />
      )}

      <ResponsiveDialog
        open={confirm !== null}
        onOpenChange={(o) => !o && !confirmPending && setConfirm(null)}
        title={confirm?.title}
        description={confirm?.description}
        footer={
          <>
            <Button
              variant={confirm?.destructive ? 'destructive' : 'default'}
              disabled={confirmPending}
              onClick={async () => {
                if (!confirm) return
                setConfirmPending(true)
                try {
                  await confirm.run()
                  setConfirm(null)
                } catch {
                  /* chybu toastuje globální handler mutací */
                } finally {
                  setConfirmPending(false)
                }
              }}
            >
              {confirmPending && <Spinner data-icon="inline-start" />}
              {confirm?.confirmLabel}
            </Button>
            <Button variant="outline" disabled={confirmPending} onClick={() => setConfirm(null)}>
              Zpět
            </Button>
          </>
        }
      />
    </>
  )
}

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 rounded-xl border bg-card p-4 text-card-foreground md:p-5">
      <div className="flex min-h-7 items-center justify-between gap-2">
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

function Party({ label, name, lines, muted }: { label: string; name: string; lines: (string | false | undefined)[]; muted?: boolean }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 text-sm">
      <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{label}</span>
      <span className={cn('font-medium', muted && 'text-foreground/80')}>{name}</span>
      {lines.filter(Boolean).map((l, i) => (
        <span key={i} className="break-words text-muted-foreground">
          {l}
        </span>
      ))}
    </div>
  )
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-right tabular-nums">{children}</dd>
    </>
  )
}

function Note({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="whitespace-pre-line">{children}</dd>
    </div>
  )
}

function LinesView({ inv, payer }: { inv: Invoice; payer: boolean }) {
  const m = (v: number) => formatMoney(v, inv.currency)
  const amountOf = (l: Invoice['lines'][number]) => (inv.prices_include_vat ? l.total : l.base)
  return (
    <>
      {/* Mobil */}
      <ul className="-my-1 divide-y md:hidden">
        {inv.lines.map((l) => (
          <li key={l.id} className="flex flex-col gap-0.5 py-2.5">
            <div className="flex items-baseline justify-between gap-3">
              <span className="min-w-0 font-medium">{l.name}</span>
              <span className="shrink-0 font-medium tabular-nums">{m(amountOf(l))}</span>
            </div>
            <span className="text-xs text-muted-foreground tabular-nums">
              {formatQuantity(l.quantity)} {l.unit_name} × {m(l.unit_price)}
              {payer && ` · DPH ${formatVatRate(l.vat_rate_bps)}`}
            </span>
          </li>
        ))}
      </ul>
      {/* Desktop */}
      <table className="hidden w-full text-sm md:table">
        <thead>
          <tr className="border-b text-xs text-muted-foreground">
            <th className="py-2 pr-2 text-left font-medium">Položka</th>
            <th className="px-2 py-2 text-right font-medium">Množství</th>
            <th className="px-2 py-2 text-right font-medium">Cena/jedn.</th>
            {payer && <th className="px-2 py-2 text-right font-medium">DPH</th>}
            <th className="py-2 pl-2 text-right font-medium">{inv.prices_include_vat ? 'Celkem s DPH' : payer ? 'Bez DPH' : 'Celkem'}</th>
          </tr>
        </thead>
        <tbody className="tabular-nums">
          {inv.lines.map((l) => (
            <tr key={l.id} className="border-b last:border-b-0">
              <td className="py-2 pr-2">{l.name}</td>
              <td className="px-2 py-2 text-right whitespace-nowrap">
                {formatQuantity(l.quantity)} {l.unit_name}
              </td>
              <td className="px-2 py-2 text-right whitespace-nowrap">{m(l.unit_price)}</td>
              {payer && <td className="px-2 py-2 text-right whitespace-nowrap">{formatVatRate(l.vat_rate_bps)}</td>}
              <td className="py-2 pl-2 text-right font-medium whitespace-nowrap">{m(amountOf(l))}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}

function PaymentRow({ p, currency, canDelete, onDelete }: { p: InvoicePayment; currency: string; canDelete: boolean; onDelete: () => void }) {
  return (
    <li className="flex items-center gap-3 py-2">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-success/12 text-success dark:bg-success/20">
        <BanknoteIcon className="size-4" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="font-medium tabular-nums">{formatMoney(p.amount, currency)}</span>
        <span className="truncate text-xs text-muted-foreground">
          {formatDate(p.paid_on)}
          {p.note && ` · ${p.note}`}
        </span>
      </div>
      {canDelete && (
        <Button variant="ghost" size="icon" aria-label="Smazat platbu" className="text-muted-foreground hover:text-destructive" onClick={onDelete}>
          <Trash2Icon />
        </Button>
      )}
    </li>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true" aria-label="Načítání">
      <Skeleton className="h-28 w-full rounded-xl" />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <Skeleton className="h-64 rounded-xl" />
        <Skeleton className="h-64 rounded-xl" />
      </div>
    </div>
  )
}

