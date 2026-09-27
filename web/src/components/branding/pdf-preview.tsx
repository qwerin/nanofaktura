import { useQuery } from '@tanstack/react-query'
import { ExternalLinkIcon, InfoIcon } from 'lucide-react'
import { useEffect, useId, useMemo, useState } from 'react'
import { api, unwrap } from '@/api/client'
import { errorMessage } from '@/api/errors'
import { keys } from '@/api/queries/keys'
import type { Account, PdfPreviewQuery } from '@/api/types'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
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

function previewUrl(slug: string, q: Required<PreviewQuery>): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? ''
  return `${base}/api/accounts/${encodeURIComponent(slug)}/pdf-preview?${new URLSearchParams(q).toString()}`
}

/**
 * Náhled PDF s ukázkovými daty (`GET /pdf-preview`) — firemní údaje, logo a podpis jsou skutečné.
 * Šablonu zatím nejde na účtu uložit (Account nemá pole) → volba slouží jen k porovnání.
 * Desktop: iframe, mobil: otevřít v nové záložce (SPEC §6 — PDF na mobilu ne v iframe).
 */
export function PdfPreview({ slug, account }: { slug: string; account: Account }) {
  const isMobile = useIsMobile()
  const [template, setTemplate] = useState<PdfTemplate>('classic')
  const [lang, setLang] = useState<PdfLanguage>(account.default_language)
  const [documentType, setDocumentType] = useState<PdfDocumentType>('invoice')
  const templateLabelId = useId()
  const langId = useId()
  const typeId = useId()
  const query = { template, lang, document_type: documentType }

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
              key={o.value}
              className="flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50 has-[[data-checked]]:border-primary has-[[data-checked]]:bg-primary/5"
            >
              <RadioGroupItem value={o.value} className="mt-0.5" />
              <span className="flex min-w-0 flex-col gap-0.5">
                <span className="text-sm font-medium">{o.label}</span>
                <span className="text-xs text-muted-foreground">{o.description}</span>
              </span>
            </label>
          ))}
        </RadioGroup>
      </div>

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
          Náhled používá ukázkové položky a odběratele, údaje vaší firmy, logo a podpis jsou skutečné. Volba šablony se
          zatím neukládá — slouží jen k porovnání vzhledu.
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
  query: Required<PreviewQuery>
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
