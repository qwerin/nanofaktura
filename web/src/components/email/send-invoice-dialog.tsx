// Odeslání dokladu e-mailem: příjemci, druh (faktura / upomínka / poděkování), text z šablony účtu
// (GET /email-templates/preview), přílohy. Backend zapíše historii a u faktury ji označí jako odeslanou.

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { PaperclipIcon, SendIcon, TriangleAlertIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/errors'
import { emailQueries, useSendInvoice } from '@/api/queries/emails'
import type { EmailKind, EmailLang, Invoice } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldContent, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { Segmented } from '@/components/recurring/segmented'
import { cn } from '@/lib/utils'
import { EMAIL_LANGS, emailKindLabels, emailLangFor, emailLangLabels, isEmailLike, sendableKinds } from './placeholders'
import { RecipientsInput } from './recipients-input'

interface SendInvoiceDialogProps {
  slug: string
  invoice: Invoice
  open: boolean
  onOpenChange: (open: boolean) => void
  initialKind?: EmailKind
}

type Edits = { subject?: string; body?: string }

export function SendInvoiceDialog({ slug, invoice, open, onOpenChange, initialKind }: SendInvoiceDialogProps) {
  const { canManageSettings } = useCurrentAccount()
  const kinds = sendableKinds(invoice)
  const [kind, setKind] = useState<EmailKind>(initialKind && kinds.includes(initialKind) ? initialKind : kinds[0] ?? 'invoice')
  const [lang, setLang] = useState<EmailLang | undefined>(undefined)
  const [to, setTo] = useState<string[] | null>(null)
  const [cc, setCc] = useState<string[] | null>(null)
  const [showCc, setShowCc] = useState(false)
  const [edits, setEdits] = useState<Record<string, Edits>>({})
  const [attachPdf, setAttachPdf] = useState<boolean | null>(null)
  const [attachIsdoc, setAttachIsdoc] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<{ to?: string; cc?: string; toIdx: number[]; ccIdx: number[] }>({ toIdx: [], ccIdx: [] })
  const [failure, setFailure] = useState<string | null>(null)
  const send = useSendInvoice(slug, invoice.id)
  const ids = { to: useId(), cc: useId(), lang: useId(), subject: useId(), body: useId(), pdf: useId(), isdoc: useId() }

  const preview = useQuery({ ...emailQueries.preview(slug, { kind, lang, invoice_id: invoice.id }), enabled: open })
  const p = preview.data
  const editKey = `${kind}:${lang ?? p?.lang ?? ''}`
  const toValue = to ?? p?.to ?? []
  const ccValue = cc ?? p?.cc ?? []
  const subject = edits[editKey]?.subject ?? p?.subject ?? ''
  const body = edits[editKey]?.body ?? p?.body ?? ''
  const pdf = attachPdf ?? kind !== 'paid_thanks'
  const edited = edits[editKey]?.subject !== undefined || edits[editKey]?.body !== undefined
  const currentLang = (lang ?? p?.lang ?? invoice.language) as EmailLang

  const setEdit = (patch: Edits) => setEdits((prev) => ({ ...prev, [editKey]: { ...prev[editKey], ...patch } }))

  const submit = () => {
    setFailure(null)
    setFieldErrors({ toIdx: [], ccIdx: [] })
    if (toValue.length === 0 && ccValue.length === 0) {
      setFieldErrors({ to: 'Zadejte aspoň jednu adresu.', toIdx: [], ccIdx: [] })
      return
    }
    const badTo = toValue.flatMap((a, i) => (isEmailLike(a) ? [] : [i]))
    const badCc = ccValue.flatMap((a, i) => (isEmailLike(a) ? [] : [i]))
    if (badTo.length || badCc.length) {
      setFieldErrors({
        to: badTo.length ? 'Opravte zvýrazněné adresy.' : undefined,
        cc: badCc.length ? 'Opravte zvýrazněné adresy.' : undefined,
        toIdx: badTo,
        ccIdx: badCc,
      })
      return
    }
    send.mutate(
      {
        kind,
        to: toValue,
        cc: ccValue,
        subject: subject.trim() || undefined,
        body: body.trim() ? body : undefined,
        attach_pdf: pdf,
        attach_isdoc: kind !== 'paid_thanks' && attachIsdoc,
      },
      {
        onSuccess: (log) => {
          toast.success(`E-mail odeslán na ${[...log.to, ...log.cc].join(', ')}`)
          onOpenChange(false)
        },
        onError: (err) => {
          if (isApiError(err) && err.status === 422 && err.problem.errors?.length) {
            const next = { toIdx: [] as number[], ccIdx: [] as number[], to: undefined as string | undefined, cc: undefined as string | undefined }
            for (const e of err.problem.errors) {
              const m = /^body\.(to|cc)(?:\[(\d+)\])?/.exec(e.location ?? '')
              if (!m) continue
              const field = m[1] as 'to' | 'cc'
              if (m[2] !== undefined) next[field === 'to' ? 'toIdx' : 'ccIdx'].push(Number(m[2]))
              next[field] = field === 'to' && !m[2] ? 'Zadejte aspoň jednu adresu.' : 'Server adresu odmítl — zkontrolujte ji.'
            }
            if (next.to || next.cc) {
              setFieldErrors(next)
              return
            }
          }
          if (isApiError(err) && err.status === 502) {
            setFailure(err.problem.detail || 'Poštovní server zprávu nepřijal.')
            return
          }
          toast.error(errorMessage(err))
        },
      },
    )
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !send.isPending && onOpenChange(o)}
      title={`Odeslat ${invoice.number} e-mailem`}
      description={invoice.client_name}
      className="sm:max-w-xl md:max-h-[calc(100dvh-2rem)] md:overflow-y-auto"
      footer={
        <>
          <Button onClick={submit} disabled={send.isPending || preview.isPending}>
            {send.isPending ? <Spinner data-icon="inline-start" /> : <SendIcon data-icon="inline-start" />}
            Odeslat
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={send.isPending}>
            Zrušit
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {failure && (
          <Alert variant="destructive">
            <TriangleAlertIcon />
            <AlertTitle>E-mail se nepodařilo odeslat</AlertTitle>
            <AlertDescription>
              <span className="break-words">{failure}</span>
              <span>Pokus je zapsaný v historii e-mailů. Zkuste to znovu, nebo zkontrolujte nastavení odesílání.</span>
            </AlertDescription>
          </Alert>
        )}

        {kinds.length > 1 && (
          <Segmented
            value={kind}
            onChange={(k) => {
              setKind(k)
              setAttachPdf(null)
            }}
            label="Druh e-mailu"
            options={kinds.map((k) => ({ value: k, label: k === 'paid_thanks' ? 'Poděkování' : emailKindLabels[k] }))}
            disabled={send.isPending}
          />
        )}

        <Field data-invalid={Boolean(fieldErrors.to) || undefined}>
          <div className="flex items-center justify-between gap-2">
            <FieldLabel htmlFor={ids.to}>Komu</FieldLabel>
            {!showCc && ccValue.length === 0 && (
              <button type="button" onClick={() => setShowCc(true)} className="text-sm font-medium text-primary hover:underline">
                + Kopie
              </button>
            )}
          </div>
          {preview.isPending ? (
            <Skeleton className="h-11 w-full md:h-8" />
          ) : (
            <RecipientsInput
              id={ids.to}
              value={toValue}
              onChange={(v) => {
                setTo(v)
                setFieldErrors((f) => ({ ...f, to: undefined, toIdx: [] }))
              }}
              placeholder="klient@firma.cz"
              invalid={Boolean(fieldErrors.to)}
              invalidIndexes={fieldErrors.toIdx}
              disabled={send.isPending}
            />
          )}
          {!preview.isPending && p && p.to.length === 0 && to === null && (
            <FieldDescription>Faktura nemá e-mail odběratele — zadejte adresu ručně.</FieldDescription>
          )}
          <FieldError>{fieldErrors.to}</FieldError>
        </Field>

        {(showCc || ccValue.length > 0) && (
          <Field data-invalid={Boolean(fieldErrors.cc) || undefined}>
            <FieldLabel htmlFor={ids.cc}>Kopie</FieldLabel>
            <RecipientsInput
              id={ids.cc}
              value={ccValue}
              onChange={(v) => {
                setCc(v)
                setFieldErrors((f) => ({ ...f, cc: undefined, ccIdx: [] }))
              }}
              invalid={Boolean(fieldErrors.cc)}
              invalidIndexes={fieldErrors.ccIdx}
              disabled={send.isPending}
            />
            <FieldError>{fieldErrors.cc}</FieldError>
          </Field>
        )}

        <Field>
          <FieldLabel id={ids.lang}>Jazyk e-mailu</FieldLabel>
          <div role="radiogroup" aria-labelledby={ids.lang} className="flex flex-wrap gap-1.5">
            {EMAIL_LANGS.map((l) => (
              <button
                key={l}
                type="button"
                role="radio"
                aria-checked={currentLang === l}
                onClick={() => setLang(l)}
                disabled={send.isPending}
                className={cn(
                  'h-11 rounded-lg border px-3 text-sm md:h-8',
                  currentLang === l ? 'border-primary bg-primary/10 font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {emailLangLabels[l]}
              </button>
            ))}
          </div>
          {currentLang !== emailLangFor(invoice.language) && (
            <FieldDescription>Doklad je v jiném jazyce — e-mail nebude ve stejném jazyce jako PDF.</FieldDescription>
          )}
        </Field>

        <Field>
          <FieldLabel htmlFor={ids.subject}>Předmět</FieldLabel>
          {preview.isPending ? (
            <Skeleton className="h-11 w-full md:h-8" />
          ) : (
            <Input id={ids.subject} value={subject} onChange={(e) => setEdit({ subject: e.target.value })} disabled={send.isPending} />
          )}
        </Field>

        <Field>
          <FieldLabel htmlFor={ids.body}>Text</FieldLabel>
          {preview.isPending ? (
            <Skeleton className="h-48 w-full" />
          ) : (
            <Textarea
              id={ids.body}
              value={body}
              onChange={(e) => setEdit({ body: e.target.value })}
              rows={8}
              className="min-h-40 text-sm leading-relaxed"
              disabled={send.isPending}
            />
          )}
          <FieldDescription>
            {edited ? (
              <button
                type="button"
                className="font-medium text-primary hover:underline"
                onClick={() => setEdits((prev) => ({ ...prev, [editKey]: {} }))}
              >
                Obnovit text ze šablony
              </button>
            ) : (
              <>
                Text ze šablony účtu
                {canManageSettings && (
                  <>
                    {' '}
                    ·{' '}
                    <Link to="/a/$slug/settings/emails" params={{ slug }} className="text-primary underline-offset-4 hover:underline">
                      upravit šablony
                    </Link>
                  </>
                )}
              </>
            )}
          </FieldDescription>
          {preview.isError && <FieldError>Text ze šablony se nepodařilo načíst: {errorMessage(preview.error)}</FieldError>}
        </Field>

        <div className="flex flex-col gap-2 rounded-lg border p-3">
          <p className="flex items-center gap-2 text-sm font-medium">
            <PaperclipIcon className="size-4 text-muted-foreground" />
            Přílohy
          </p>
          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor={ids.pdf}>PDF dokladu</FieldLabel>
            </FieldContent>
            <Switch id={ids.pdf} checked={pdf} onCheckedChange={(v) => setAttachPdf(v)} disabled={send.isPending} />
          </Field>
          {kind !== 'paid_thanks' && (
            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor={ids.isdoc}>ISDOC</FieldLabel>
                <FieldDescription>Strojově čitelná faktura pro účetní programy.</FieldDescription>
              </FieldContent>
              <Switch id={ids.isdoc} checked={attachIsdoc} onCheckedChange={setAttachIsdoc} disabled={send.isPending} />
            </Field>
          )}
        </div>

        {kind === 'invoice' && invoice.status === 'open' && (
          <p className="text-xs text-muted-foreground">Po odeslání se faktura označí jako odeslaná.</p>
        )}
      </div>
    </ResponsiveDialog>
  )
}
