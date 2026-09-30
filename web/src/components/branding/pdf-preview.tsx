import { useQuery } from '@tanstack/react-query'
import { ExternalLinkIcon, InfoIcon, SaveIcon } from 'lucide-react'
import { useEffect, useId, useMemo, useState } from 'react'
import { api, unwrap } from '@/api/client'
import { errorMessage } from '@/api/errors'
import { useUpdateAccount } from '@/api/queries/accounts'
import { keys } from '@/api/queries/keys'
import type { Account, PdfPreviewQuery } from '@/api/types'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from 'sonner'
import { useDebouncedValue } from '@/components/expense/use-debounced-value'
import { useIsMobile } from '@/hooks/use-mobile'
import { cn } from '@/lib/utils'

type PreviewQuery = PdfPreviewQuery
type PdfTemplate = NonNullable<PreviewQuery['template']>
type PdfLanguage = NonNullable<PreviewQuery['lang']>
type PdfDocumentType = NonNullable<PreviewQuery['document_type']>

const templateOptions: { value: PdfTemplate; label: string; description: string }[] = [
  { value: 'classic', label: 'Klasická', description: 'Tradiční rozvržení s tabulkou' },
  { value: 'modern', label: 'Moderní', description: 'Výrazné záhlaví s barvou' },
  { value: 'minimal', label: 'Minimalistická', description: 'Jen to podstatné, hodně vzduchu' },
]

const languageOptions: { value: PdfLanguage; label: string }[] = [
  { value: 'cs', label: 'Čeština' },
  { value: 'en', label: 'Angličtina' },
  { value: 'sk', label: 'Slovenština' },
  { value: 'de', label: 'Němčina' },
]

const documentTypeOptions: { value: PdfDocumentType; label: string }[] = [
  { value: 'invoice', label: 'Faktura' },
  { value: 'proforma', label: 'Zálohová faktura' },
  { value: 'correction', label: 'Opravný daňový doklad' },
]

const HEX = /^#[0-9A-Fa-f]{6}$/

function previewUrl(slug: string, q: PreviewQuery): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? ''
  const params = new URLSearchParams(Object.entries(q).filter((e): e is [string, string] => Boolean(e[1])))
  return `${base}/api/accounts/${encodeURIComponent(slug)}/pdf-preview?${params.toString()}`
}

/**
 * Vzhled PDF (šablona, barva, QR, patička — ukládá se na účet a platí pro všechna PDF)
 * a náhled s ukázkovými daty (`GET /pdf-preview`, neuložené hodnoty jdou v query).
 * Desktop: iframe, mobil: otevřít v nové záložce (SPEC §6 — PDF na mobilu ne v iframe).
 */
export function PdfPreview({ slug, account, canEdit = false }: { slug: string; account: Account; canEdit?: boolean }) {
  const isMobile = useIsMobile()
  const update = useUpdateAccount(slug)
  const [template, setTemplate] = useState<PdfTemplate>(account.pdf_template)
  const [accent, setAccent] = useState(account.pdf_accent)
  const [showQr, setShowQr] = useState(account.pdf_show_qr)
  const [footer, setFooter] = useState(account.pdf_footer)
  const [lang, setLang] = useState<PdfLanguage>(account.default_language)
  const [documentType, setDocumentType] = useState<PdfDocumentType>('invoice')
  const templateLabelId = useId()
  const langId = useId()
  const typeId = useId()
  const accentId = useId()
  const qrId = useId()
  const footerId = useId()
  const accentValid = accent === '' || HEX.test(accent)
  // text inputs re-render the PDF only after typing pauses
  const previewAccent = useDebouncedValue(accentValid ? accent : '', 400)
  const previewFooter = useDebouncedValue(footer, 600)
  const query: PreviewQuery = {
    template,
    lang,
    document_type: documentType,
    accent: previewAccent,
    show_qr: showQr ? 'true' : 'false',
    footer: previewFooter,
  }
  const dirty =
    template !== account.pdf_template ||
    accent !== account.pdf_accent ||
    showQr !== account.pdf_show_qr ||
    footer !== account.pdf_footer
  const save = () =>
    update.mutate(
      { pdf_template: template, pdf_accent: accent, pdf_show_qr: showQr, pdf_footer: footer },
      {
        onSuccess: () => toast.success('Vzhled dokladů uložen'),
        onError: (err) => toast.error(errorMessage(err)),
      },
    )

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-2">
        <span id={templateLabelId} className="text-sm font-medium">
          Šablona
        </span>
        <RadioGroup
          aria-labelledby={templateLabelId}
          value={template}
          onValueChange={(v) => setTemplate(v as PdfTemplate)}
          className="grid gap-2 sm:grid-cols-3"
        >
          {templateOptions.map((o) => (
            <label
              data-disabled={!canEdit || undefined}
              key={o.value}
              className="flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50 has-[[data-checked]]:border-primary has-[[data-checked]]:bg-primary/5"
            >
              <RadioGroupItem value={o.value} className="mt-0.5" disabled={!canEdit} />
              <span className="flex min-w-0 flex-col gap-0.5">
                <span className="text-sm font-medium">{o.label}</span>
                <span className="text-xs text-muted-foreground">{o.description}</span>
              </span>
            </label>
          ))}
        </RadioGroup>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-2">
          <Label htmlFor={accentId}>Barva akcentu</Label>
          <div className="flex items-center gap-2">
            <input
              type="color"
              aria-label="Vybrat barvu"
              value={accentValid && accent ? accent : '#1f6feb'}
              onChange={(e) => setAccent(e.target.value)}
              disabled={!canEdit}
              className="size-11 shrink-0 cursor-pointer rounded-md border bg-transparent p-1 disabled:cursor-not-allowed"
            />
            <Input
              id={accentId}
              value={accent}
              placeholder="Výchozí barva šablony"
              onChange={(e) => setAccent(e.target.value.trim())}
              aria-invalid={!accentValid || undefined}
              disabled={!canEdit}
              className="h-11"
            />
            {accent && canEdit && (
              <Button variant="ghost" size="sm" onClick={() => setAccent('')}>
                Výchozí
              </Button>
            )}
          </div>
          {!accentValid && <p className="text-sm text-destructive">Zadejte barvu ve tvaru #RRGGBB.</p>}
        </div>
        <div className="flex min-h-11 items-center justify-between gap-3 rounded-lg border p-3 sm:self-end">
          <Label htmlFor={qrId} className="flex flex-col items-start gap-0.5">
            <span>QR platba</span>
            <span className="text-xs font-normal text-muted-foreground">Na fakturách v Kč s českým účtem</span>
          </Label>
          <Switch id={qrId} checked={showQr} onCheckedChange={setShowQr} disabled={!canEdit} />
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor={footerId}>Vlastní patička</Label>
        <Textarea
          id={footerId}
          value={footer}
          maxLength={500}
          rows={2}
          placeholder="Např. Jsme zapsáni v obchodním rejstříku… / Děkujeme za spolupráci"
          onChange={(e) => setFooter(e.target.value)}
          disabled={!canEdit}
        />
      </div>

      {canEdit && (
        <div className="flex justify-end">
          <Button onClick={save} disabled={!dirty || !accentValid || update.isPending} className="w-full sm:w-auto">
            <SaveIcon data-icon="inline-start" />
            Uložit vzhled
          </Button>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        <PreviewSelect id={langId} label="Jazyk" value={lang} options={languageOptions} onChange={setLang} />
        <PreviewSelect
          id={typeId}
          label="Typ dokladu"
          value={documentType}
          options={documentTypeOptions}
          onChange={setDocumentType}
        />
      </div>

      <Alert>
        <InfoIcon />
        <AlertDescription>
          Náhled používá ukázkové položky a odběratele, údaje vaší firmy, logo a podpis jsou skutečné. Jazyk a typ
          dokladu slouží jen k náhledu; šablona, barva, QR a patička se po uložení použijí na všech PDF.
        </AlertDescription>
      </Alert>

      {isMobile ? (
        <Button
          nativeButton={false}
          size="lg"
          className="w-full"
          render={<a href={previewUrl(slug, query)} target="_blank" rel="noopener" />}
        >
          <ExternalLinkIcon data-icon="inline-start" />
          Otevřít náhled PDF
        </Button>
      ) : (
        <PdfFrame
          slug={slug}
          query={query}
          version={`${account.updated_at}-${account.logo_attachment_id ?? 0}-${account.stamp_attachment_id ?? 0}`}
        />
      )}
    </div>
  )
}

function PreviewSelect<V extends string>({
  id,
  label,
  value,
  options,
  onChange,
}: {
  id: string
  label: string
  value: V
  options: { value: V; label: string }[]
  onChange: (v: V) => void
}) {
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Select items={options} value={value} onValueChange={(v) => v && onChange(v as V)}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function PdfFrame({
  slug,
  query,
  version,
}: {
  slug: string
  query: PreviewQuery
  /** Mění se se změnou firemních údajů, loga či podpisu → nové vykreslení. */
  version: string
}) {
  const pdf = useQuery({
    queryKey: keys.pdfPreview(slug, { ...query, version }),
    queryFn: ({ signal }) =>
      unwrap(
        api.GET('/api/accounts/{slug}/pdf-preview', {
          params: { path: { slug }, query },
          parseAs: 'blob',
          signal,
        }),
      ) as Promise<Blob>,
    staleTime: Infinity,
    placeholderData: (prev) => prev,
  })
  const url = useMemo(() => (pdf.data ? URL.createObjectURL(pdf.data) : null), [pdf.data])
  useEffect(() => () => (url ? URL.revokeObjectURL(url) : undefined), [url])

  if (pdf.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>Náhled se nepodařilo vytvořit: {errorMessage(pdf.error)}</AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="overflow-hidden rounded-xl border bg-muted">
      {url ? (
        <iframe
          key={url}
          src={`${url}#view=FitH`}
          title="Náhled PDF faktury"
          className={cn('aspect-[1/1.414] w-full bg-white transition-opacity', pdf.isFetching && 'opacity-60')}
        />
      ) : (
        <Skeleton className="aspect-[1/1.414] w-full rounded-none" />
      )}
    </div>
  )
}
