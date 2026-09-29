import { CircleAlertIcon, CopyIcon, FileUpIcon, LinkIcon, SparklesIcon, UploadIcon, XIcon, type LucideIcon } from 'lucide-react'
import { useId, useRef, useState, type DragEvent } from 'react'
import { useImportStatement } from '@/api/queries/bank'
import type { BankAccount, BankImportResult } from '@/api/types'
import { formatFileSize } from '@/components/attachments/files'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import { importErrorText, MAX_STATEMENT_BYTES, STATEMENT_ACCEPT, STATEMENT_FORMATS, statementFormatLabel } from './format'

const AUTO = '__auto'
const formatItems = [{ value: AUTO, label: 'Rozpoznat automaticky' }, ...STATEMENT_FORMATS]

interface ImportDialogProps {
  slug: string
  open: boolean
  onOpenChange: (open: boolean) => void
  accounts: BankAccount[]
  /** Předvybraný účet (z karty účtu). */
  bankAccountId?: number
  /** „Zobrazit návrhy“ po importu. */
  onShowUnmatched?: (bankAccountId: number) => void
}

/** Import výpisu (GPC/ABO, CSV bank, Fio JSON): výběr souboru, formát, souhrn výsledku. */
export function ImportStatementDialog(props: ImportDialogProps) {
  return (
    <ResponsiveDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title="Importovat výpis"
      description="Nahrajte výpis z internetového bankovnictví. Už nahrané pohyby se přeskočí."
    >
      {props.open && <ImportBody key={props.bankAccountId ?? 'any'} {...props} />}
    </ResponsiveDialog>
  )
}

function ImportBody({ slug, accounts, bankAccountId, onOpenChange, onShowUnmatched }: ImportDialogProps) {
  const id = useId()
  const importM = useImportStatement(slug)
  const [accountId, setAccountId] = useState<number | undefined>(bankAccountId ?? (accounts.length === 1 ? accounts[0]?.id : undefined))
  const [format, setFormat] = useState<string>(AUTO)
  const [file, setFile] = useState<File | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<BankImportResult | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  const choose = (f: File | undefined) => {
    setError(null)
    if (!f) return
    if (f.size > MAX_STATEMENT_BYTES) {
      setError('Soubor je větší než 10 MB.')
      return
    }
    if (f.size === 0) {
      setError('Soubor je prázdný.')
      return
    }
    setFile(f)
  }

  const submit = () => {
    if (!file || !accountId) return
    setError(null)
    importM.mutate(
      { bankAccountId: accountId, file, format: format === AUTO ? undefined : format },
      { onSuccess: setResult, onError: (err) => setError(importErrorText(err)) },
    )
  }

  if (result) {
    return <ImportSummary result={result} onClose={() => onOpenChange(false)} onShow={accountId && onShowUnmatched ? () => onShowUnmatched(accountId) : undefined} />
  }

  const accountItems = accounts.map((a) => ({ value: String(a.id), label: `${a.name} · ${a.number || a.iban}` }))

  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      {accounts.length > 1 && (
        <Field>
          <FieldLabel htmlFor={`${id}-account`}>Bankovní účet</FieldLabel>
          <Select items={accountItems} value={accountId ? String(accountId) : null} onValueChange={(v) => setAccountId(v ? Number(v) : undefined)}>
            <SelectTrigger id={`${id}-account`} className="w-full">
              <SelectValue placeholder="Vyberte účet…" />
            </SelectTrigger>
            <SelectContent>
              {accountItems.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}

      <div
        onDragOver={(e: DragEvent) => {
          if (!e.dataTransfer.types.includes('Files')) return
          e.preventDefault()
          setDragOver(true)
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e: DragEvent) => {
          e.preventDefault()
          setDragOver(false)
          choose(e.dataTransfer.files[0])
        }}
      >
        {file ? (
          <div className="flex items-center gap-3 rounded-xl border bg-muted/40 p-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <FileUpIcon className="size-5" />
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{file.name}</p>
              <p className="text-xs text-muted-foreground">{formatFileSize(file.size)}</p>
            </div>
            <Button type="button" variant="ghost" size="icon" aria-label="Odebrat soubor" onClick={() => setFile(null)} disabled={importM.isPending}>
              <XIcon />
            </Button>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => input.current?.click()}
            className={cn(
              'flex w-full flex-col items-center gap-2 rounded-xl border-2 border-dashed px-4 py-8 text-center transition-colors hover:bg-muted/50',
              dragOver && 'border-primary bg-primary/5',
            )}
          >
            <UploadIcon className="size-6 text-muted-foreground" />
            <span className="text-sm font-medium">
              <span className="md:hidden">Vybrat soubor s výpisem</span>
              <span className="max-md:hidden">Přetáhněte soubor sem nebo klikněte</span>
            </span>
            <span className="text-xs text-muted-foreground">GPC/ABO, CSV (Fio, ČSOB, KB, Air Bank) nebo Fio JSON · max 10 MB</span>
          </button>
        )}
        <input
          ref={input}
          type="file"
          accept={STATEMENT_ACCEPT}
          className="sr-only"
          tabIndex={-1}
          aria-hidden="true"
          onChange={(e) => {
            choose(e.target.files?.[0])
            e.target.value = ''
          }}
        />
      </div>

      <Field>
        <FieldLabel htmlFor={`${id}-format`}>Formát</FieldLabel>
        <Select items={formatItems} value={format} onValueChange={(v) => setFormat(v ?? AUTO)}>
          <SelectTrigger id={`${id}-format`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {formatItems.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FieldDescription>Většinou stačí automatické rozpoznání.</FieldDescription>
      </Field>

      {error && (
        <p className="flex items-start gap-1.5 text-sm text-destructive" role="alert">
          <CircleAlertIcon className="mt-0.5 size-4 shrink-0" />
          {error}
        </p>
      )}

      <div className="flex flex-col-reverse gap-2 md:flex-row md:justify-end">
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Zrušit
        </Button>
        <Button type="submit" disabled={!file || !accountId || importM.isPending}>
          {importM.isPending ? <Spinner data-icon="inline-start" /> : <UploadIcon data-icon="inline-start" />}
          Importovat
        </Button>
      </div>
    </form>
  )
}

function ImportSummary({ result: r, onClose, onShow }: { result: BankImportResult; onClose: () => void; onShow?: () => void }) {
  const stats: { label: string; value: number; icon: LucideIcon; tone: string }[] = [
    { label: 'Nových pohybů', value: r.imported, icon: FileUpIcon, tone: 'text-primary bg-primary/10' },
    { label: 'Spárováno automaticky', value: r.matched, icon: LinkIcon, tone: 'text-success bg-success/12' },
    { label: 'S návrhem k potvrzení', value: r.suggestions, icon: SparklesIcon, tone: 'text-info bg-info/12' },
    { label: 'Duplicit přeskočeno', value: r.duplicates, icon: CopyIcon, tone: 'text-muted-foreground bg-muted' },
  ]
  return (
    <div className="flex flex-col gap-5" aria-live="polite">
      <p className="text-sm text-muted-foreground">
        Výpis načten ({statementFormatLabel(r.format)}).{' '}
        {r.imported === 0 && r.duplicates > 0 ? 'Všechny pohyby už byly nahrané dříve.' : ''}
      </p>
      <dl className="grid grid-cols-2 gap-2">
        {stats.map((s) => (
          <div key={s.label} className="flex items-center gap-3 rounded-xl border p-3">
            <span className={cn('flex size-9 shrink-0 items-center justify-center rounded-lg', s.tone)}>
              <s.icon className="size-4" />
            </span>
            <div className="flex min-w-0 flex-col-reverse">
              <dt className="mt-1 text-xs leading-tight text-muted-foreground">{s.label}</dt>
              <dd className="text-xl leading-none font-semibold tabular-nums">{s.value}</dd>
            </div>
          </div>
        ))}
      </dl>
      <div className="flex flex-col-reverse gap-2 md:flex-row md:justify-end">
        <Button variant="outline" onClick={onClose}>
          Hotovo
        </Button>
        {onShow && r.imported - r.matched > 0 && (
          <Button
            onClick={() => {
              onShow()
              onClose()
            }}
          >
            Zobrazit nespárované
          </Button>
        )}
      </div>
    </div>
  )
}
