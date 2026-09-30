import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import {
  ClipboardCopyIcon,
  FilePlusIcon,
  GlobeIcon,
  MailIcon,
  PencilIcon,
  PhoneIcon,
  Trash2Icon,
  type LucideIcon,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { subjectQueries } from '@/api/queries/subjects'
import type { Subject } from '@/api/types'
import { VatStatusCard } from '@/components/bank/vat-status-card'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'
import { CopyIconButton } from '@/components/subject/copy-icon-button'
import { DeleteSubjectDialog } from '@/components/subject/delete-subject-dialog'
import { subjectTypeLabels } from '@/components/subject/labels'
import { SubjectInvoices } from '@/components/subject/subject-invoices'
import { Badge } from '@/components/ui/badge'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatIban } from '@/lib/bank'
import { addressLines, displayWeb, formatAddress, telHref, webHref } from '@/lib/contact'
import { HistorySection } from '@/components/events/timeline'
import { parseId } from '@/lib/params'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/subjects/$subjectId/')({
  params: {
    parse: ({ subjectId }) => ({ subjectId: parseId(subjectId) }),
    stringify: ({ subjectId }) => ({ subjectId: String(subjectId) }),
  },
  loader: ({ context, params }) =>
    context.queryClient.ensureQueryData(subjectQueries.detail(params.slug, params.subjectId)),
  head: ({ loaderData }) => ({ meta: [{ title: `${loaderData?.name ?? 'Kontakt'} · NanoFaktura` }] }),
  component: SubjectDetailPage,
})

/** Fakturační údaje jako text pro vložení do e-mailu / zprávy. */
function billingText(s: Subject): string {
  return [
    s.name,
    ...addressLines(s),
    s.registration_no && `IČO: ${s.registration_no}`,
    s.vat_no && `DIČ: ${s.vat_no}`,
  ]
    .filter(Boolean)
    .join('\n')
}

function SubjectDetailPage() {
  const { slug, subjectId } = Route.useParams()
  const navigate = useNavigate()
  const subject = useQuery(subjectQueries.detail(slug, subjectId))
  const [deleteOpen, setDeleteOpen] = useState(false)
  const { canEdit } = useCurrentAccount()
  const s = subject.data

  const copyBilling = async () => {
    if (!s) return
    try {
      await navigator.clipboard.writeText(billingText(s))
      toast.success('Fakturační údaje zkopírovány')
    } catch {
      toast.error('Kopírování se nezdařilo.')
    }
  }

  const editActions: PageAction[] = [
    {
      label: 'Upravit',
      icon: PencilIcon,
      primary: true,
      variant: 'outline',
      render: <Link to="/a/$slug/subjects/$subjectId/edit" params={{ slug, subjectId }} />,
    },
    {
      label: 'Nová faktura',
      icon: FilePlusIcon,
      variant: 'default',
      render: <Link to="/a/$slug/invoices/new" params={{ slug }} search={{ subject_id: subjectId }} />,
    },
  ]

  return (
    <>
      <PageHeader
        title={s?.name ?? 'Kontakt'}
        back={{ to: '/a/$slug/subjects', params: { slug } }}
        actions={[
          ...(canEdit ? editActions : []),
          { label: 'Kopírovat fakturační údaje', icon: ClipboardCopyIcon, onClick: () => void copyBilling() },
          ...(canEdit
            ? [{ label: 'Smazat', icon: Trash2Icon, variant: 'destructive' as const, onClick: () => setDeleteOpen(true) }]
            : []),
        ]}
      />
      <PageBody>
        {subject.isError ? (
          <PageError error={subject.error} reset={() => void subject.refetch()} />
        ) : !s ? (
          <FormSkeleton fields={5} className="max-w-xl" />
        ) : (
          <div className="grid gap-6 lg:grid-cols-[minmax(0,26rem)_minmax(0,1fr)] lg:items-start">
            <div className="flex flex-col gap-4">
              <ContactHero subject={s} />
              <DetailCard subject={s} />
              <VatStatusCard slug={slug} subject={s} />
              <HistorySection subjectType="subject" subjectId={s.id} />
            </div>
            <SubjectInvoices slug={slug} subjectId={s.id} canCreate={canEdit} />
          </div>
        )}
      </PageBody>

      {s && (
        <DeleteSubjectDialog
          slug={slug}
          subject={s}
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
          onDeleted={() => void navigate({ to: '/a/$slug/subjects', params: { slug }, replace: true })}
        />
      )}
    </>
  )
}

/** Hlavička kontaktu + rychlé akce (volat / psát / web) — velké cíle pro prst. */
function ContactHero({ subject: s }: { subject: Subject }) {
  const quick: { label: string; href: string; icon: LucideIcon; external?: boolean }[] = []
  if (s.phone) quick.push({ label: 'Zavolat', href: telHref(s.phone), icon: PhoneIcon })
  if (s.email) quick.push({ label: 'E-mail', href: `mailto:${s.email}`, icon: MailIcon })
  if (s.web) quick.push({ label: 'Web', href: webHref(s.web), icon: GlobeIcon, external: true })

  return (
    <div className="rounded-xl border bg-card p-4 md:p-5">
      <div className="flex items-start gap-3">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-full bg-primary/10 text-lg font-semibold text-primary">
          {s.name.trim().charAt(0).toUpperCase() || '?'}
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-lg leading-tight font-semibold break-words">{s.name}</h2>
          {s.full_name && <p className="mt-0.5 text-sm text-muted-foreground">{s.full_name}</p>}
          <div className="mt-2 flex flex-wrap gap-1.5">
            <Badge variant="secondary">{subjectTypeLabels[s.type]}</Badge>
            {s.custom_id && <Badge variant="outline">ID {s.custom_id}</Badge>}
          </div>
        </div>
      </div>
      {quick.length > 0 && (
        <div className="mt-4 grid grid-cols-3 gap-2">
          {quick.map((q) => (
            <a
              key={q.label}
              href={q.href}
              target={q.external ? '_blank' : undefined}
              rel={q.external ? 'noopener noreferrer' : undefined}
              className="flex min-h-16 flex-col items-center justify-center gap-1 rounded-lg bg-muted/60 px-2 py-2 text-xs font-medium text-foreground transition-colors hover:bg-muted active:bg-muted md:min-h-14"
            >
              <q.icon className="size-5 text-primary" />
              {q.label}
            </a>
          ))}
        </div>
      )}
    </div>
  )
}

function DetailCard({ subject: s }: { subject: Subject }) {
  const address = formatAddress(s)
  const hasPayment = s.bank_account || s.iban || s.swift_bic || s.due_days != null

  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <dl>
        <Row label="IČO" value={s.registration_no} copy mono />
        <Row label="DIČ" value={s.vat_no} copy mono />
        {s.local_vat_no && <Row label="IČ DPH" value={s.local_vat_no} copy mono />}
        <Row
          label="Adresa"
          value={address}
          copy
          display={
            address && (
              <span className="flex flex-col">
                {addressLines(s).map((l) => (
                  <span key={l}>{l}</span>
                ))}
              </span>
            )
          }
        />
        <Row
          label="E-mail"
          value={s.email}
          copy
          display={
            s.email && (
              <a href={`mailto:${s.email}`} className="break-all text-primary hover:underline">
                {s.email}
              </a>
            )
          }
        />
        {s.email_copy && <Row label="Kopie e-mailu" value={s.email_copy} copy />}
        <Row
          label="Telefon"
          value={s.phone}
          display={
            s.phone && (
              <a href={telHref(s.phone)} className="text-primary hover:underline">
                {s.phone}
              </a>
            )
          }
        />
        <Row
          label="Web"
          value={s.web}
          display={
            s.web && (
              <a href={webHref(s.web)} target="_blank" rel="noopener noreferrer" className="break-all text-primary hover:underline">
                {displayWeb(s.web)}
              </a>
            )
          }
        />
      </dl>
      {hasPayment && (
        <>
          <h3 className="border-t bg-muted/40 px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            Platební údaje
          </h3>
          <dl>
            {s.bank_account && <Row label="Účet" value={s.bank_account} copy mono />}
            {s.iban && <Row label="IBAN" value={s.iban} display={formatIban(s.iban)} copy mono />}
            {s.swift_bic && <Row label="SWIFT" value={s.swift_bic} copy mono />}
            {s.due_days != null && <Row label="Splatnost" value={`${s.due_days} dní`} />}
          </dl>
        </>
      )}
      {s.note && (
        <div className="border-t px-4 py-3">
          <h3 className="text-xs font-medium text-muted-foreground">Poznámka</h3>
          <p className="mt-1 text-sm whitespace-pre-line">{s.note}</p>
        </div>
      )}
    </div>
  )
}

function Row({
  label,
  value,
  display,
  copy,
  mono,
}: {
  label: string
  value: string
  display?: ReactNode
  copy?: boolean
  mono?: boolean
}) {
  return (
    <div className="flex min-h-12 items-center gap-3 border-b px-4 py-2 last:border-b-0">
      <dt className="w-24 shrink-0 self-start pt-0.5 text-sm text-muted-foreground">{label}</dt>
      <dd className={cn('min-w-0 flex-1 text-sm', mono && 'font-mono tabular-nums', !value && 'text-muted-foreground')}>
        {value ? (display ?? value) : '—'}
      </dd>
      {copy && value ? <CopyIconButton value={value} label={`Kopírovat ${label}`} className="-my-1 -mr-2" /> : null}
    </div>
  )
}
