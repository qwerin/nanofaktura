import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { ArchiveRestoreIcon, CircleAlertIcon, FileArchiveIcon, TriangleAlertIcon, UploadIcon, XIcon } from 'lucide-react'
import { useId, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateAccount } from '@/api/queries/accounts'
import { type ImportResult, useImportBackup } from '@/api/queries/backup'
import { TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatBytes, importWarningText, looksLikeBackup, progressPercent } from '@/lib/backup'
import { cn } from '@/lib/utils'

const schema = z.object({
  name: z.string().trim().min(1, 'Zadejte název'),
})
type Values = z.infer<typeof schema>
type Mode = 'new' | 'restore'

/**
 * Založení dalšího účtu (firmy / OSVČ) — prázdného, nebo obnoveného ze zálohy (SPEC §7.16).
 * Po vytvoření přepne na nový účet.
 */
export function CreateAccountDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const create = useCreateAccount()
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: '' } })
  const [mode, setMode] = useState<Mode>('new')
  const [result, setResult] = useState<ImportResult | null>(null)
  const restore = useRestoreState()

  const close = () => {
    onOpenChange(false)
    form.reset()
    restore.reset()
    setResult(null)
    setMode('new')
  }

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const account = await create.mutateAsync(values)
      toast.success(`Účet „${account.name}“ založen`)
      close()
      await navigate({ to: '/a/$slug/settings/company', params: { slug: account.slug } })
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  const onRestore = async () => {
    const res = await restore.submit()
    if (!res) return
    toast.success(`Účet „${res.account.name}“ obnoven ze zálohy`)
    if (res.warnings.length === 0) {
      close()
      await navigate({ to: '/a/$slug', params: { slug: res.account.slug } })
    } else {
      setResult(res)
    }
  }

  const goToImported = async () => {
    if (!result) return
    const slug = result.account.slug
    close()
    await navigate({ to: '/a/$slug', params: { slug } })
  }

  const footer = result ? (
    <Button onClick={() => void goToImported()}>Přejít do účtu</Button>
  ) : mode === 'new' ? (
    <>
      <Button type="submit" form="create-account-form" disabled={create.isPending}>
        {create.isPending && <Spinner data-icon="inline-start" />}
        Založit účet
      </Button>
      <Button variant="outline" onClick={close}>
        Zrušit
      </Button>
    </>
  ) : (
    <>
      <Button onClick={() => void onRestore()} disabled={!restore.file || restore.pending}>
        {restore.pending ? <Spinner data-icon="inline-start" /> : <ArchiveRestoreIcon data-icon="inline-start" />}
        Obnovit účet
      </Button>
      <Button variant="outline" onClick={close} disabled={restore.pending}>
        Zrušit
      </Button>
    </>
  )

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => (o ? onOpenChange(true) : !restore.pending && close())}
      title={result ? 'Účet obnoven' : 'Nový účet'}
      description={
        result
          ? `„${result.account.name}“ je připravený. Zkontrolujte prosím:`
          : 'Další firma nebo OSVČ, za kterou budete vystavovat doklady.'
      }
      footer={footer}
    >
      {result ? (
        <ul className="flex flex-col gap-3 text-sm">
          {result.warnings.map((w) => (
            <li key={w.code} className="flex gap-2">
              <TriangleAlertIcon className="mt-0.5 size-4 shrink-0 text-warning" />
              {importWarningText(w)}
            </li>
          ))}
        </ul>
      ) : (
        <div className="flex flex-col gap-5">
          <Tabs value={mode} onValueChange={(v) => setMode(v as Mode)}>
            <TabsList className="h-11! w-full md:h-8!">
              <TabsTrigger value="new" className="flex-1" disabled={restore.pending}>
                Prázdný účet
              </TabsTrigger>
              <TabsTrigger value="restore" className="flex-1" disabled={create.isPending}>
                Obnovit ze zálohy
              </TabsTrigger>
            </TabsList>
          </Tabs>
          {mode === 'new' ? (
            <form id="create-account-form" onSubmit={onSubmit} noValidate>
              <TextField
                control={form.control}
                name="name"
                label="Název firmy / jméno"
                autoComplete="organization"
                enterKeyHint="done"
              />
            </form>
          ) : (
            <RestoreForm state={restore} />
          )}
        </div>
      )}
    </ResponsiveDialog>
  )
}

function useRestoreState() {
  const importM = useImportBackup()
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [progress, setProgress] = useState<{ loaded: number; total: number } | null>(null)
  const [error, setError] = useState<string | null>(null)

  const choose = (f: File | undefined) => {
    setError(null)
    if (!f) return
    if (!looksLikeBackup(f)) {
      setError('Vyberte ZIP soubor se zálohou (nanofaktura-….zip).')
      return
    }
    setFile(f)
  }

  const submit = async (): Promise<ImportResult | null> => {
    if (!file) return null
    setError(null)
    setProgress({ loaded: 0, total: file.size })
    try {
      return await importM.mutateAsync({ file, name, onProgress: (loaded, total) => setProgress({ loaded, total }) })
    } catch (err) {
      setError(errorMessage(err))
      return null
    } finally {
      setProgress(null)
    }
  }

  const reset = () => {
    setFile(null)
    setName('')
    setProgress(null)
    setError(null)
    importM.reset()
  }

  return { file, setFile, name, setName, progress, error, choose, submit, reset, pending: importM.isPending }
}

function RestoreForm({ state }: { state: ReturnType<typeof useRestoreState> }) {
  const id = useId()
  const input = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const { file, progress, pending } = state
  const pct = progress ? progressPercent(progress.loaded, progress.total) : null

  return (
    <div
      className="flex flex-col gap-5"
      onDragOver={(e) => {
        e.preventDefault()
        setDragOver(true)
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setDragOver(false)
        if (!pending) state.choose(e.dataTransfer.files?.[0])
      }}
    >
      <p className="text-sm text-muted-foreground">
        Ze zálohy vznikne <strong>nový účet</strong>, jehož budete vlastníkem. Pravidelné faktury, upomínky a webhooky se
        obnoví vypnuté.
      </p>
      {file ? (
        <div className="flex items-center gap-3 rounded-xl border px-3 py-3">
          <FileArchiveIcon className="size-5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{file.name}</p>
            <p className="text-xs text-muted-foreground">{formatBytes(file.size)}</p>
          </div>
          <Button type="button" variant="ghost" size="icon" aria-label="Odebrat soubor" onClick={() => state.setFile(null)} disabled={pending}>
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
            <span className="md:hidden">Vybrat soubor se zálohou</span>
            <span className="max-md:hidden">Přetáhněte ZIP sem nebo klikněte</span>
          </span>
          <span className="text-xs text-muted-foreground">Soubor nanofaktura-….zip z Nastavení → Záloha a přenos</span>
        </button>
      )}
      <input
        ref={input}
        type="file"
        accept=".zip,application/zip"
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => {
          state.choose(e.target.files?.[0])
          e.target.value = ''
        }}
      />

      <Field>
        <FieldLabel htmlFor={`${id}-name`}>Název nového účtu</FieldLabel>
        <Input
          id={`${id}-name`}
          value={state.name}
          onChange={(e) => state.setName(e.target.value)}
          maxLength={200}
          autoComplete="organization"
          disabled={pending}
        />
        <FieldDescription>Nepovinné — jinak se použije název ze zálohy.</FieldDescription>
      </Field>

      {progress && (
        <div className="flex flex-col gap-1.5" role="status" aria-live="polite">
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary transition-[width]"
              style={{ width: `${pct === null || pct >= 100 ? 100 : pct}%` }}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {pct !== null && pct < 100
              ? `Nahrávám… ${pct} % (${formatBytes(progress.loaded)} z ${formatBytes(progress.total)})`
              : 'Obnovuji účet…'}
          </p>
        </div>
      )}

      {state.error && (
        <p className="flex items-start gap-1.5 text-sm text-destructive" role="alert">
          <CircleAlertIcon className="mt-0.5 size-4 shrink-0" />
          {state.error}
        </p>
      )}
    </div>
  )
}
