import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { FilePlusIcon, InfoIcon, RepeatIcon, Trash2Icon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { bankAccountQueries } from '@/api/queries/bank-accounts'
import { recurringQueries } from '@/api/queries/recurring'
import { subjectQueries } from '@/api/queries/subjects'
import { templateQueries, useCreateInvoiceFromTemplate, useDeleteTemplate } from '@/api/queries/templates'
import type { InvoiceTemplate } from '@/api/types'
import { documentTypeLabels, documentTypeShortLabels } from '@/components/invoice/status'
import { PageBody, PageHeader, type PageAction } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { renderDatePlaceholders } from '@/components/recurring/period'
import { Segmented } from '@/components/recurring/segmented'
import { TemplateForm } from '@/components/recurring/template-form'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { parseISODate, todayISO } from '@/lib/date'
import { parseId } from '@/lib/params'

export const Route = createFileRoute('/a/$slug/templates/$templateId')({
  params: {
    parse: ({ templateId }) => ({ templateId: parseId(templateId) }),
    stringify: ({ templateId }) => ({ templateId: String(templateId) }),
  },
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(templateQueries.detail(params.slug, params.templateId)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Šablona faktury · NanoFaktura' }] }),
  component: TemplatePage,
})

function TemplatePage() {
  const { slug, templateId } = Route.useParams()
  const template = useQuery(templateQueries.detail(slug, templateId))
  const account = useQuery(accountQueries.detail(slug))
  const back = { to: '/a/$slug/templates', params: { slug } } as const

  const error = template.error ?? account.error
  if (error) {
    return (
      <>
        <PageHeader title="Šablona" back={back} />
        <PageError error={error} reset={() => void Promise.all([template.refetch(), account.refetch()])} />
      </>
    )
  }
  if (!template.data || !account.data) {
    return (
      <>
        <PageHeader title="Šablona" back={back} />
        <PageBody>
          <FormSkeleton fields={8} className="max-w-3xl" />
        </PageBody>
      </>
    )
  }
  return <TemplateDetail slug={slug} template={template.data} />
}

function TemplateDetail({ slug, template }: { slug: string; template: InvoiceTemplate }) {
  const navigate = useNavigate()
  const { canEdit } = useCurrentAccount()
  const account = useQuery(accountQueries.detail(slug))
  const subject = useQuery(subjectQueries.detail(slug, template.subject_id))
  const usedBy = useQuery(recurringQueries.list(slug, { template_id: template.id }))
  const remove = useDeleteTemplate(slug)
  const [issueOpen, setIssueOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const subjectValue = useMemo(
    () =>
      subject.data
        ? { id: subject.data.id, name: subject.data.name, registration_no: subject.data.registration_no, city: subject.data.city }
        : { id: template.subject_id, name: subject.isError ? 'Smazaný kontakt' : '…' },
    [subject.data, subject.isError, template.subject_id],
  )
  const uses = usedBy.data?.items ?? []

  const actions: PageAction[] = canEdit
    ? [
        { label: 'Vytvořit fakturu', icon: FilePlusIcon, primary: true, variant: 'default', onClick: () => setIssueOpen(true) },
        {
          label: 'Nastavit opakování',
          icon: RepeatIcon,
          render: <Link to="/a/$slug/recurring/new" params={{ slug }} search={{ template_id: template.id }} />,
        },
        { label: 'Smazat šablonu', icon: Trash2Icon, overflow: true, variant: 'destructive', onClick: () => setDeleteOpen(true) },
      ]
    : []

  return (
    <>
      <PageHeader
        title={template.name}
        description={`Šablona · ${documentTypeLabels[template.document_type]}`}
        back={{ to: '/a/$slug/templates', params: { slug } }}
        actions={actions}
      />
      <PageBody>
        {uses.length > 0 && (
          <Alert className="mb-4">
            <InfoIcon />
            <AlertDescription>
              <span>
                Šablonu používá{' '}
                {uses.map((r, i) => (
                  <span key={r.id}>
                    {i > 0 && ', '}
                    <Link to="/a/$slug/recurring/$recurringId" params={{ slug, recurringId: r.id }} className="font-medium text-foreground underline-offset-4 hover:underline">
                      {r.name}
                    </Link>
                  </span>
                ))}
                . Změny se projeví v příštích vystavených fakturách.
              </span>
            </AlertDescription>
          </Alert>
        )}
        {subject.isPending ? (
          <FormSkeleton fields={8} className="max-w-3xl" />
        ) : (
          account.data && (
            <TemplateForm
              key={`${template.id}-${template.updated_at}`}
              slug={slug}
              account={account.data}
              template={template}
              subject={subjectValue}
              readOnly={!canEdit}
              onSaved={() => {}}
            />
          )
        )}
      </PageBody>

      <IssueDialog
        slug={slug}
        template={template}
        open={issueOpen}
        onOpenChange={setIssueOpen}
        onIssued={(id) => void navigate({ to: '/a/$slug/invoices/$invoiceId', params: { slug, invoiceId: id } })}
      />

      <ResponsiveDialog
        open={deleteOpen}
        onOpenChange={(o) => !remove.isPending && setDeleteOpen(o)}
        title={`Smazat šablonu „${template.name}“?`}
        description={
          uses.length > 0
            ? 'Šablonu používá pravidelná faktura — nejdřív ji smažte nebo přepněte na jinou šablonu.'
            : 'Šablona se trvale odstraní. Už vystavené faktury zůstanou beze změny.'
        }
        footer={
          <>
            <Button
              variant="destructive"
              disabled={remove.isPending || uses.length > 0}
              onClick={() =>
                remove.mutate(template.id, {
                  onSuccess: () => {
                    toast.success('Šablona smazána')
                    setDeleteOpen(false)
                    void navigate({ to: '/a/$slug/templates', params: { slug }, replace: true })
                  },
                })
              }
            >
              {remove.isPending && <Spinner data-icon="inline-start" />}
              Smazat
            </Button>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={remove.isPending}>
              Zpět
            </Button>
          </>
        }
      />
    </>
  )
}

/** Vystavení faktury ze šablony: datum (určuje i placeholdery) a typ dokladu. */
function IssueDialog({
  slug,
  template,
  open,
  onOpenChange,
  onIssued,
}: {
  slug: string
  template: InvoiceTemplate
  open: boolean
  onOpenChange: (o: boolean) => void
  onIssued: (invoiceId: number) => void
}) {
  const issue = useCreateInvoiceFromTemplate(slug, template.id)
  const [issuedOn, setIssuedOn] = useState(todayISO)
  const [docType, setDocType] = useState<InvoiceTemplate['document_type']>(template.document_type)
  const valid = parseISODate(issuedOn) !== null
  const firstLine = template.lines[0]?.name ?? ''
  const lang = template.language === 'en' ? 'en' : 'cs'

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !issue.isPending && onOpenChange(o)}
      title="Vytvořit fakturu ze šablony"
      description={`Vystaví se nový doklad pro ${template.name}. Pak ho můžete upravit nebo odeslat.`}
      footer={
        <>
          <Button
            disabled={!valid || issue.isPending}
            onClick={() =>
              issue.mutate(
                { issued_on: issuedOn, document_type: docType },
                {
                  onSuccess: (inv) => {
                    toast.success(`${documentTypeLabels[inv.document_type]} ${inv.number} vystavena`)
                    onOpenChange(false)
                    onIssued(inv.id)
                  },
                },
              )
            }
          >
            {issue.isPending && <Spinner data-icon="inline-start" />}
            Vystavit
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={issue.isPending}>
            Zrušit
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Field>
          <FieldLabel htmlFor="issue-date">Datum vystavení</FieldLabel>
          <Input id="issue-date" type="date" value={issuedOn} onChange={(e) => setIssuedOn(e.target.value)} />
          {valid && firstLine.includes('{') && (
            <FieldDescription>
              První položka: <span className="font-medium text-foreground">{renderDatePlaceholders(firstLine, issuedOn, lang)}</span>
            </FieldDescription>
          )}
        </Field>
        <Field>
          <FieldLabel>Typ dokladu</FieldLabel>
          <Segmented
            value={docType}
            onChange={setDocType}
            label="Typ dokladu"
            options={(['invoice', 'proforma'] as const).map((t) => ({ value: t, label: documentTypeShortLabels[t] }))}
          />
        </Field>
      </div>
    </ResponsiveDialog>
  )
}
