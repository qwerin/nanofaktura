import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  BanIcon,
  BanknoteIcon,
  CopyIcon,
  DownloadIcon,
  FileMinusIcon,
  FileCodeIcon,
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
  BellRingIcon,
  FileStackIcon,
  MailIcon,
  FileCheckIcon,
  Link2Icon,
  StampIcon,
  PencilLineIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { hasErrorCode } from '@/api/errors'
import {
  invoicePdfUrl,
  invoiceQueries,
  useDeleteInvoice,
  useDeletePayment,
  useDuplicateInvoice,
  useInvoiceAction,
} from '@/api/queries/invoices'
import type { EmailKind, Invoice, InvoiceAction, InvoiceDeposit, InvoicePayment, RelatedDocument } from '@/api/types'
import { EmailHistory } from '@/components/email/email-history'
import { HistorySection } from '@/components/events/timeline'
import { sendableKinds } from '@/components/email/placeholders'
import { SendInvoiceDialog } from '@/components/email/send-invoice-dialog'
import { AddPaymentDialog } from '@/components/invoice/add-payment-dialog'
import { CorrectionDialog } from '@/components/invoice/correction-dialog'
import { FinalInvoiceDialog } from '@/components/invoice/final-invoice-dialog'
import { DueText } from '@/components/invoice/due-text'
import { InvoiceNumber } from '@/components/invoice/invoice-number'
import { StatusBadge } from '@/components/invoice/status-badge'
import {
  allowedActions,
  documentTypeLabels,
  invoiceLabel,
  isSettledProforma,
  issuedMessage,
  paymentMethodLabels,
  statusLabels,
  type UiAction,
} from '@/components/invoice/status'
import { TotalsPanel } from '@/components/invoice/totals-panel'
import { useCanEditDocuments } from '@/components/invoice/use-can-edit'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { SaveAsTemplateDialog } from '@/components/recurring/save-as-template-dialog'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
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
  const [send, setSend] = useState<{ open: boolean; kind: EmailKind; key: number }>({ open: false, kind: 'invoice', key: 0 })
  const [saveTemplateOpen, setSaveTemplateOpen] = useState(false)
  const [correctionOpen, setCorrectionOpen] = useState(false)
  const [finalOpen, setFinalOpen] = useState(false)

  const action = useInvoiceAction(slug, inv.id)
  const duplicate = useDuplicateInvoice(slug, inv.id)
  const remove = useDeleteInvoice(slug)
  const deletePayment = useDeletePayment(slug, inv.id)

  const allowed = canEdit ? allowedActions({ ...inv, payment_count: inv.payments.length }) : new Set<UiAction>()
  const payer = inv.your_vat_mode !== 'non_vat_payer'
  const taxDocument = inv.document_type === 'tax_document'
  const draft = inv.status === 'draft'
  const settled = isSettledProforma(inv)
  const finalInvoice = settled ? inv.related_documents.find((d) => d.document_type === 'invoice') : undefined
  const pdfUrl = invoicePdfUrl(slug, inv.id)
  const pdfName = `faktura-${invoiceLabel(inv, { lower: true }).replace(/[^A-Za-z0-9._-]/g, '-')}.pdf`
  // Koncept veřejný odkaz nemá (klient by dostal 404).
  const publicUrl = inv.public_token && !draft ? `${window.location.origin}/p/${inv.public_token}` : null

  const run = (a: InvoiceAction, msg: string) =>
    action.mutate(a, { onSuccess: () => toast.success(msg) })

  const ask = (c: Confirm) => setConfirm(c)
  const issueDraft = () => action.mutate('issue', { onSuccess: (issued) => toast.success(issuedMessage(issued)) })

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

  const openSend = (kind: EmailKind) => setSend((s) => ({ open: true, kind, key: s.key + 1 }))
  const canSend = canEdit && inv.status !== 'cancelled' && !draft
  const canRemind = canSend && sendableKinds(inv).includes('reminder')
  const notSent = !inv.sent_at && (inv.status === 'open' || inv.status === 'overdue')
  const sendActions: PageAction[] = canSend
    ? [
        {
          label: 'Odeslat e-mailem',
          icon: MailIcon,
          // U dosud neodeslané faktury je odeslání hlavní akce.
          primary: notSent,
          variant: notSent ? 'default' : 'outline',
          overflow: !notSent,
          onClick: () => openSend('invoice'),
        },
        ...(canRemind
          ? [
              {
                label: 'Poslat upomínku',
                icon: BellRingIcon,
                primary: !notSent && inv.status === 'overdue',
                variant: 'outline',
                overflow: notSent || inv.status !== 'overdue',
                onClick: () => openSend('reminder'),
              } satisfies PageAction,
            ]
          : []),
      ]
    : []

  const act = (key: UiAction, a: Omit<PageAction, 'label'> & { label: string }): PageAction[] => (allowed.has(key) ? [a] : [])

  const actions: PageAction[] = [
    ...act('issue', {
      label: 'Vystavit',
      icon: StampIcon,
      primary: true,
      variant: 'default',
      disabled: action.isPending,
      onClick: issueDraft,
    }),
    ...sendActions,
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
    // ISDOC (strojově čitelná faktura pro účetní software)
    ...(draft
      ? []
      : [{ label: 'Stáhnout ISDOC', icon: FileCodeIcon, overflow: true, render: <a href={pdfUrl.replace(/\/pdf$/, '/isdoc')} download={pdfName.replace(/\.pdf$/, '.isdoc')} /> }]),
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
    ...act('final_invoice', {
      label: 'Vystavit vyúčtování',
      icon: FileCheckIcon,
      overflow: true,
      onClick: () => setFinalOpen(true),
    }),
    ...(allowed.has('duplicate')
      ? [
          {
            label: 'Duplikovat',
            icon: CopyIcon,
            overflow: true,
            onClick: () =>
              duplicate.mutate(undefined, {
                onSuccess: (d) => {
                  toast.success(d.status === 'draft' ? 'Vytvořena kopie (koncept)' : `Vytvořena kopie ${invoiceLabel(d)}`)
                  void navigate({ to: '/a/$slug/invoices/$invoiceId/edit', params: { slug, invoiceId: d.id } })
                },
              }),
          },
        ]
      : []),
    ...(canEdit && inv.document_type !== 'correction' && !taxDocument
      ? [{ label: 'Uložit jako šablonu', icon: FileStackIcon, overflow: true, onClick: () => setSaveTemplateOpen(true) }]
      : []),
    ...act('correction', {
      label: 'Vystavit opravný doklad',
      icon: FileMinusIcon,
      overflow: true,
      onClick: () => setCorrectionOpen(true),
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
          run: () =>
            action.mutateAsync('cancel').then(
              () => toast.success('Doklad stornován'),
              (err: unknown) => {
                // Odeslaný daňový doklad nejde stornovat — nabídneme rovnou dobropis (chybu toastuje globální handler).
                if (hasErrorCode(err, 'correction_required') && allowed.has('correction')) setCorrectionOpen(true)
                else throw err
              },
            ),
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
          title: `Smazat ${invoiceLabel(inv, { lower: true })}?`,
          description: draft
            ? 'Koncept se trvale odstraní.'
            : 'Doklad se trvale odstraní. Pokud už byl odeslán klientovi, raději ho stornujte.',
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

  const title = <InvoiceNumber number={inv.number} />
  const client = [inv.client_street, [inv.client_zip, inv.client_city].filter(Boolean).join(' ')].filter(Boolean)
  const busy = action.isPending || duplicate.isPending

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
        {draft && (
          <DraftNotice canIssue={allowed.has('issue')} locked={Boolean(inv.locked_at)} pending={action.isPending} onIssue={issueDraft} />
        )}

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
              {draft ? 'Datum vystavení' : 'Vystaveno'} {formatDate(inv.issued_on)} · splatnost {formatDate(inv.due_on)}{' '}
              <DueText invoice={inv} className="whitespace-nowrap" />
            </p>
          </div>
          <div className="flex items-end justify-between gap-6 md:flex-col md:items-end md:gap-1">
            <div className="md:text-right">
              <p className="text-xs text-muted-foreground">Celkem</p>
              <p className="text-2xl font-semibold tracking-tight tabular-nums">{formatMoney(inv.total, inv.currency)}</p>
            </div>
            {finalInvoice ? (
              <p className="text-sm text-muted-foreground md:text-right">
                Vyúčtováno fakturou{' '}
                <Link
                  to="/a/$slug/invoices/$invoiceId"
                  params={{ slug, invoiceId: finalInvoice.id }}
                  className="font-medium text-primary underline-offset-4 hover:underline"
                >
                  {finalInvoice.number}
                </Link>
              </p>
            ) : (
              !taxDocument &&
              inv.status !== 'cancelled' &&
              inv.remaining_amount !== 0 &&
              inv.remaining_amount !== inv.total && (
                <p className="text-sm text-muted-foreground md:text-right">
                  zbývá <span className="font-medium text-foreground tabular-nums">{formatMoney(inv.remaining_amount, inv.currency)}</span>
                </p>
              )
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
                    inv.client_local_vat_no && `IČ DPH ${inv.client_local_vat_no}`,
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
                remaining={settled || taxDocument ? undefined : inv.remaining_amount}
              />
              {settled && finalInvoice && inv.remaining_amount !== 0 && (
                <p className="text-xs text-muted-foreground">
                  Zbylých {formatMoney(inv.remaining_amount, inv.currency)} je k úhradě na faktuře {finalInvoice.number}.
                </p>
              )}
              {taxDocument && (
                <p className="text-xs text-muted-foreground">
                  Doklad o přijaté záloze — není pohledávkou, DPH z něj se odečte ve vyúčtování.
                </p>
              )}
            </Section>

            {inv.deposits.length > 0 && <DepositsSection slug={slug} deposits={inv.deposits} currency={inv.currency} payer={payer} />}

            {inv.related_documents.length > 0 && (
              <Section title="Související doklady">
                <ul className="-my-1 divide-y">
                  {inv.related_documents.map((d) => (
                    <RelatedRow key={d.id} slug={slug} doc={d} current={inv} />
                  ))}
                </ul>
              </Section>
            )}

            {!draft && (
              <>
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
                          slug={slug}
                          currency={inv.currency}
                          canDelete={canEdit}
                          onDelete={() =>
                            ask({
                              title: 'Smazat platbu?',
                              description: `Platba ${formatMoney(p.amount, inv.currency)} z ${formatDate(p.paid_on)} se odstraní a stav faktury se přepočítá.${
                                p.tax_document_id ? ' Smaže se i daňový doklad k této platbě.' : ''
                              }`,
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

                <Section
                  title="E-maily"
                  action={
                    canSend && (
                      <Button variant="ghost" size="sm" onClick={() => openSend(canRemind && !notSent && inv.status === 'overdue' ? 'reminder' : 'invoice')} className="max-md:h-9">
                        <MailIcon data-icon="inline-start" />
                        Odeslat
                      </Button>
                    )
                  }
                >
                  <EmailHistory slug={slug} invoiceId={inv.id} />
                </Section>
              </>
            )}

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
                  <Info label={inv.document_type === 'correction' ? 'Opravuje' : 'Zálohová faktura'}>
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
            <HistorySection subjectType="invoice" subjectId={inv.id} />
          </div>
        </div>
      </PageBody>

      {allowed.has('add_payment') && (
        <AddPaymentDialog slug={slug} invoice={inv} open={paymentOpen} onOpenChange={setPaymentOpen} />
      )}

      {canSend && (
        <SendInvoiceDialog
          key={send.key}
          slug={slug}
          invoice={inv}
          open={send.open}
          initialKind={send.kind}
          onOpenChange={(open) => setSend((s) => ({ ...s, open }))}
        />
      )}
      {allowed.has('correction') && (
        <CorrectionDialog slug={slug} invoice={inv} open={correctionOpen} onOpenChange={setCorrectionOpen} />
      )}
      {allowed.has('final_invoice') && (
        <FinalInvoiceDialog slug={slug} invoice={inv} open={finalOpen} onOpenChange={setFinalOpen} />
      )}
      {canEdit && <SaveAsTemplateDialog slug={slug} invoice={inv} open={saveTemplateOpen} onOpenChange={setSaveTemplateOpen} />}

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

/** Koncept: vysvětlení + hlavní akce „Vystavit“ (na mobilu přes celou šířku). */
function DraftNotice({ canIssue, locked, pending, onIssue }: { canIssue: boolean; locked: boolean; pending: boolean; onIssue: () => void }) {
  return (
    <section className="mb-4 flex flex-col gap-3 rounded-xl border border-dashed border-muted-foreground/40 bg-muted/40 p-4 md:flex-row md:items-center md:justify-between md:p-5">
      <div className="flex min-w-0 gap-3">
        <PencilLineIcon className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
        <div className="flex min-w-0 flex-col gap-0.5">
          <p className="font-medium">Koncept — zatím nevystaveno</p>
          <p className="text-sm text-muted-foreground">
            Nemá číslo ani variabilní symbol a nikam se nepočítá. Číslo dostane až při vystavení; datum vystavení v minulosti
            se posune na dnešek.{locked && ' Před vystavením ho odemkněte.'}
          </p>
        </div>
      </div>
      {canIssue && (
        <Button className="max-md:h-11 max-md:w-full md:shrink-0" disabled={pending} onClick={onIssue}>
          {pending ? <Spinner data-icon="inline-start" /> : <StampIcon data-icon="inline-start" />}
          Vystavit
        </Button>
      )}
    </section>
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

/** Popisek dokladu, který odkazuje na ten zobrazený (dobropis, daňový doklad k platbě, vyúčtování). */
function relatedLabel(doc: RelatedDocument, current: Invoice): string {
  if (current.document_type === 'proforma' && doc.document_type === 'invoice') return 'Vyúčtováno fakturou'
  if (doc.document_type === 'tax_document') return 'Daňový doklad k platbě'
  return documentTypeLabels[doc.document_type]
}

function RelatedRow({ slug, doc, current }: { slug: string; doc: RelatedDocument; current: Invoice }) {
  const status = doc.status in statusLabels ? statusLabels[doc.status as keyof typeof statusLabels] : doc.status
  return (
    <li className="flex items-center gap-3 py-2">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Link2Icon className="size-4" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="text-xs text-muted-foreground">{relatedLabel(doc, current)}</span>
        <Link
          to="/a/$slug/invoices/$invoiceId"
          params={{ slug, invoiceId: doc.id }}
          className="truncate font-medium text-primary underline-offset-4 hover:underline"
        >
          <InvoiceNumber number={doc.number} />
        </Link>
      </div>
      <div className="flex shrink-0 flex-col items-end">
        <span className="text-sm font-medium tabular-nums">{formatMoney(doc.total, current.currency)}</span>
        <span className="text-xs text-muted-foreground">{status}</span>
      </div>
    </li>
  )
}

/** Odpočet záloh na vyúčtovací faktuře: daňové doklady k přijatým platbám a jejich DPH po sazbách. */
function DepositsSection({ slug, deposits, currency, payer }: { slug: string; deposits: InvoiceDeposit[]; currency: string; payer: boolean }) {
  const m = (v: number) => formatMoney(v, currency)
  const sum = deposits.reduce((s, d) => s + d.total, 0)
  return (
    <Section title="Odpočet záloh">
      <ul className="-my-1 divide-y">
        {deposits.map((d) => (
          <li key={d.tax_document_id} className="flex flex-col gap-1 py-2 text-sm">
            <div className="flex items-baseline justify-between gap-3">
              <Link
                to="/a/$slug/invoices/$invoiceId"
                params={{ slug, invoiceId: d.tax_document_id }}
                className="truncate font-medium text-primary underline-offset-4 hover:underline"
              >
                {d.number}
              </Link>
              <span className="shrink-0 font-medium tabular-nums">−{m(d.total)}</span>
            </div>
            <span className="text-xs text-muted-foreground">DUZP {formatDate(d.taxable_fulfillment_due)}</span>
            {payer &&
              d.vat_recap.map((r) => (
                <span key={r.vat_rate_bps} className="flex justify-between gap-3 text-xs text-muted-foreground tabular-nums">
                  <span>
                    DPH {formatVatRate(r.vat_rate_bps)} ze základu {m(r.base)}
                  </span>
                  <span>{m(r.vat)}</span>
                </span>
              ))}
          </li>
        ))}
      </ul>
      {deposits.length > 1 && (
        <p className="flex justify-between text-sm font-medium tabular-nums">
          <span>Zálohy celkem</span>
          <span>−{m(sum)}</span>
        </p>
      )}
      <p className="text-xs text-muted-foreground">DPH ze záloh už bylo odvedeno, ve vyúčtování se odečte.</p>
    </Section>
  )
}

function PaymentRow({
  p,
  slug,
  currency,
  canDelete,
  onDelete,
}: {
  p: InvoicePayment
  slug: string
  currency: string
  canDelete: boolean
  onDelete: () => void
}) {
  const takenOver = Boolean(p.source_payment_id)
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
          {takenOver && ' · převzato ze zálohy'}
        </span>
        {p.tax_document_id && (
          <Link
            to="/a/$slug/invoices/$invoiceId"
            params={{ slug, invoiceId: p.tax_document_id }}
            className="text-xs text-primary underline-offset-4 hover:underline"
          >
            Daňový doklad k platbě
          </Link>
        )}
      </div>
      {canDelete &&
        (takenOver ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <span
                  tabIndex={0}
                  aria-label="Platba převzatá ze zálohové faktury"
                  className="inline-flex rounded-md focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
                >
                  <Button variant="ghost" size="icon" aria-label="Smazat platbu" disabled className="pointer-events-none text-muted-foreground">
                    <Trash2Icon />
                  </Button>
                </span>
              }
            />
            <TooltipContent className="max-w-64">Platba převzatá ze zálohové faktury — smažte ji na zálohové faktuře.</TooltipContent>
          </Tooltip>
        ) : (
          <Button variant="ghost" size="icon" aria-label="Smazat platbu" className="text-muted-foreground hover:text-destructive" onClick={onDelete}>
            <Trash2Icon />
          </Button>
        ))}
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

