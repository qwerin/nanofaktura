import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { CircleAlertIcon, LockIcon, PencilIcon, PlusIcon, StarIcon, Trash2Icon } from 'lucide-react'
import { useId, useRef, useState } from 'react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/errors'
import {
  numberFormatQueries,
  useCreateNumberFormat,
  useDeleteNumberFormat,
  useUpdateNumberFormat,
} from '@/api/queries/number-formats'
import type { NumberFormat, NumberFormatDocumentType } from '@/api/types'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { SettingsPage } from '@/components/settings-page'
import { ListSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import {
  insertAt,
  MAX_FORMAT_LENGTH,
  numberFormatError,
  numberFormatPeriod,
  renderNumberFormat,
} from '@/lib/number-format'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/a/$slug/settings/number-formats')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(numberFormatQueries.list(params.slug)),
  head: () => ({ meta: [{ title: 'Číselné řady · NanoFaktura' }] }),
  component: NumberFormatsPage,
})

const groups: { type: NumberFormatDocumentType; title: string; description: string; suggestion: string }[] = [
  { type: 'invoice', title: 'Faktury', description: 'Běžné daňové doklady.', suggestion: 'FV{YY}{NNNN}' },
  { type: 'proforma', title: 'Zálohové faktury', description: 'Výzvy k platbě zálohy.', suggestion: 'ZF{YY}{NNNN}' },
  { type: 'correction', title: 'Opravné doklady', description: 'Dobropisy a opravné daňové doklady.', suggestion: 'OD{YY}{NNNN}' },
  { type: 'expense', title: 'Náklady', description: 'Interní čísla přijatých dokladů.', suggestion: 'N{YY}{NNNN}' },
]

const periodLabels = {
  month: 'Číslování začíná každý měsíc znovu od 1.',
  year: 'Číslování začíná každý rok znovu od 1.',
  never: 'Číslování pokračuje bez nulování.',
} as const

type DialogState = { mode: 'create'; type: NumberFormatDocumentType } | { mode: 'edit'; format: NumberFormat } | null

function NumberFormatsPage() {
  const { slug } = Route.useParams()
  const { canManageSettings } = useCurrentAccount()
  const list = useQuery(numberFormatQueries.list(slug))
  const [dialog, setDialog] = useState<DialogState>(null)
  const [deleting, setDeleting] = useState<NumberFormat | null>(null)

  return (
    <SettingsPage
      title="Číselné řady"
      description={
        <>
          Formát čísel dokladů. Číslo se přidělí při vytvoření dokladu podle data vystavení, např.{' '}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">{'{YYYY}-{NNNN}'}</code> → 2026-0001.
        </>
      }
    >
      {!canManageSettings && (
        <Alert className="mb-6 max-w-2xl">
          <LockIcon />
          <AlertDescription>Číselné řady může měnit jen vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}

      {list.isError ? (
        <PageError error={list.error} reset={() => void list.refetch()} />
      ) : list.isPending ? (
        <ListSkeleton rows={4} />
      ) : (
        <div className="flex max-w-3xl flex-col gap-6">
          {groups.map((g) => {
            const formats = list.data.filter((f) => f.document_type === g.type)
            return (
              <section key={g.type} aria-labelledby={`nf-${g.type}`}>
                <div className="mb-2 flex items-end justify-between gap-3">
                  <div>
                    <h2 id={`nf-${g.type}`} className="text-base font-semibold tracking-tight">
                      {g.title}
                    </h2>
                    <p className="text-sm text-muted-foreground">{g.description}</p>
                  </div>
                  {canManageSettings && (
                    <Button variant="ghost" className="shrink-0" onClick={() => setDialog({ mode: 'create', type: g.type })}>
                      <PlusIcon data-icon="inline-start" />
                      Přidat<span className="max-sm:hidden"> řadu</span>
                    </Button>
                  )}
                </div>
                {formats.length === 0 ? (
                  <p className="rounded-xl border border-dashed px-4 py-5 text-sm text-muted-foreground">
                    Zatím bez řady — první přidaná bude výchozí.
                  </p>
                ) : (
                  <ul className="overflow-hidden rounded-xl border bg-card">
                    {formats.map((f) => (
                      <FormatRow
                        key={f.id}
                        slug={slug}
                        format={f}
                        canManage={canManageSettings}
                        onEdit={() => setDialog({ mode: 'edit', format: f })}
                        onDelete={() => setDeleting(f)}
                      />
                    ))}
                  </ul>
                )}
              </section>
            )
          })}
        </div>
      )}

      <FormatDialog slug={slug} state={dialog} onClose={() => setDialog(null)} />
      <DeleteFormatDialog slug={slug} format={deleting} onClose={() => setDeleting(null)} />
    </SettingsPage>
  )
}

function FormatRow({
  slug,
  format: f,
  canManage,
  onEdit,
  onDelete,
}: {
  slug: string
  format: NumberFormat
  canManage: boolean
  onEdit: () => void
  onDelete: () => void
}) {
  const preview = useQuery(numberFormatQueries.preview(slug, f))
  const update = useUpdateNumberFormat(slug)
  const period = numberFormatPeriod(f.format)

  return (
    <li className="flex items-start gap-2 border-b py-3 pr-2 pl-4 last:border-b-0 sm:items-center sm:gap-3 sm:pr-3">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <code className="rounded-md bg-muted px-2 py-0.5 font-mono text-sm">{f.format}</code>
          {f.is_default && (
            <Badge className="border-transparent bg-success/10 text-success">
              <StarIcon data-icon="inline-start" />
              Výchozí
            </Badge>
          )}
        </div>
        <p className="text-sm text-muted-foreground">
          Příští číslo:{' '}
          {preview.data ? (
            <span className="font-mono font-medium text-foreground">{preview.data}</span>
          ) : preview.isError ? (
            '—'
          ) : (
            <Skeleton className="inline-block h-4 w-20 align-middle" />
          )}
          {period && <span className="max-sm:block sm:before:content-['_·_']">{periodLabels[period]}</span>}
        </p>
      </div>
      {canManage && (
        <div className="-my-1 flex shrink-0 items-center sm:gap-1">
          {!f.is_default && (
            <Button
              variant="ghost"
              aria-label="Nastavit jako výchozí"
              title="Nastavit jako výchozí"
              className="max-sm:size-11 max-sm:px-0"
              disabled={update.isPending}
              onClick={() =>
                update.mutate(
                  { id: f.id, body: { is_default: true } },
                  { onSuccess: () => toast.success(`Řada ${f.format} je nyní výchozí`) },
                )
              }
            >
              {update.isPending ? <Spinner data-icon="inline-start" /> : <StarIcon data-icon="inline-start" />}
              <span className="max-sm:hidden">Nastavit výchozí</span>
            </Button>
          )}
          <Button variant="ghost" size="icon" aria-label={`Upravit řadu ${f.format}`} onClick={onEdit}>
            <PencilIcon />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Smazat řadu ${f.format}`}
            title={f.is_default ? 'Výchozí řadu nelze smazat' : undefined}
            disabled={f.is_default}
            onClick={onDelete}
            className="text-destructive hover:text-destructive"
          >
            <Trash2Icon />
          </Button>
        </div>
      )}
    </li>
  )
}

const placeholders = [
  { token: '{YYYY}', label: 'Rok', hint: '2026' },
  { token: '{YY}', label: 'Rok (2 číslice)', hint: '26' },
  { token: '{MM}', label: 'Měsíc', hint: '03' },
  { token: '{NNNN}', label: 'Pořadí', hint: '0001' },
]

/** Česká hláška pro chyby serveru u formátu. */
function formatServerError(err: unknown): string {
  if (isApiError(err) && err.status === 409) return 'Tato řada už pro daný typ dokladu existuje.'
  if (isApiError(err) && err.status === 422) return 'Neplatný formát čísla.'
  return errorMessage(err)
}

function FormatDialog({ slug, state, onClose }: { slug: string; state: DialogState; onClose: () => void }) {
  const editing = state?.mode === 'edit' ? state.format : undefined
  const type = editing?.document_type ?? (state?.mode === 'create' ? state.type : 'invoice')
  const group = groups.find((g) => g.type === type)!
  return (
    <ResponsiveDialog
      open={state !== null}
      onOpenChange={(o) => !o && onClose()}
      title={editing ? 'Upravit číselnou řadu' : `Nová řada · ${group.title}`}
      description="Poskládejte formát z textu a zástupných symbolů."
    >
      {state && (
        <FormatForm
          key={editing?.id ?? `new-${type}`}
          slug={slug}
          type={type}
          editing={editing}
          initial={editing?.format ?? group.suggestion}
          onDone={onClose}
        />
      )}
    </ResponsiveDialog>
  )
}

function FormatForm({
  slug,
  type,
  editing,
  initial,
  onDone,
}: {
  slug: string
  type: NumberFormatDocumentType
  editing?: NumberFormat
  initial: string
  onDone: () => void
}) {
  const id = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const [value, setValue] = useState(initial)
  const [makeDefault, setMakeDefault] = useState(false)
  const [touched, setTouched] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)
  const create = useCreateNumberFormat(slug)
  const update = useUpdateNumberFormat(slug)
  const pending = create.isPending || update.isPending

  const format = value.trim()
  const error = numberFormatError(format)
  const now = new Date()
  const sample1 = renderNumberFormat(format, now, 1)
  const sample2 = renderNumberFormat(format, now, 2)
  const period = numberFormatPeriod(format)
  const shownError = serverError ?? (touched ? error : null)

  const insert = (token: string) => {
    const el = inputRef.current
    const start = el?.selectionStart ?? value.length
    const end = el?.selectionEnd ?? start
    const next = insertAt(value, token, start, end)
    setValue(next.value)
    setServerError(null)
    requestAnimationFrame(() => {
      el?.focus()
      el?.setSelectionRange(next.cursor, next.cursor)
    })
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setTouched(true)
    if (error) return
    try {
      if (editing) {
        if (format !== editing.format) await update.mutateAsync({ id: editing.id, body: { format } })
        toast.success('Řada uložena')
      } else {
        await create.mutateAsync({ document_type: type, format, is_default: makeDefault || undefined })
        toast.success('Řada přidána')
      }
      onDone()
    } catch (err) {
      setServerError(formatServerError(err))
    }
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-5">
      <Field data-invalid={shownError ? true : undefined}>
        <FieldLabel htmlFor={id}>Formát</FieldLabel>
        <Input
          id={id}
          ref={inputRef}
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
            setServerError(null)
          }}
          onBlur={() => setTouched(true)}
          maxLength={MAX_FORMAT_LENGTH}
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          enterKeyHint="done"
          className="font-mono"
          aria-invalid={shownError ? true : undefined}
        />
        <div className="flex flex-wrap gap-2" role="group" aria-label="Vložit zástupný symbol">
          {placeholders.map((p) => (
            <button
              key={p.token}
              type="button"
              onClick={() => insert(p.token)}
              // Tlačítko nesmí vzít fokus inputu — jinak ztratíme pozici kurzoru.
              onMouseDown={(e) => e.preventDefault()}
              className="inline-flex h-11 items-center gap-1.5 rounded-full border bg-background px-3 text-sm transition-colors hover:bg-muted active:bg-muted md:h-8"
            >
              <code className="font-mono font-medium text-primary">{p.token}</code>
              <span className="text-muted-foreground">{p.label}</span>
            </button>
          ))}
        </div>
        <FieldDescription>
          {'{N}'} až {'{NNNNNN}'} = pořadové číslo, počet N určuje doplnění nulami.
        </FieldDescription>
        {shownError && <FieldError>{shownError}</FieldError>}
      </Field>

      <div
        className={cn(
          'rounded-xl border px-4 py-3',
          sample1 ? 'bg-muted/40' : 'border-dashed text-muted-foreground',
        )}
        aria-live="polite"
      >
        <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Náhled</p>
        {sample1 ? (
          <>
            <p className="mt-1 font-mono text-lg font-semibold break-all">
              {sample1}
              <span className="text-sm font-normal text-muted-foreground">, {sample2}, …</span>
            </p>
            {period && <p className="mt-1 text-sm text-muted-foreground">{periodLabels[period]}</p>}
            {editing && (
              <p className="mt-1 text-xs text-muted-foreground">
                Náhled ukazuje první čísla — skutečné pokračuje podle již vystavených dokladů.
              </p>
            )}
          </>
        ) : (
          <p className="mt-1 flex items-center gap-1.5 text-sm">
            <CircleAlertIcon className="size-4" />
            {error}
          </p>
        )}
      </div>

      {!editing && (
        <label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm">
          <input
            type="checkbox"
            checked={makeDefault}
            onChange={(e) => setMakeDefault(e.target.checked)}
            className="size-5 accent-(--primary)"
          />
          Nastavit jako výchozí řadu
        </label>
      )}

      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button type="button" variant="outline" onClick={onDone}>
          Zrušit
        </Button>
        <Button type="submit" disabled={pending}>
          {pending && <Spinner data-icon="inline-start" />}
          {editing ? 'Uložit' : 'Přidat řadu'}
        </Button>
      </div>
    </form>
  )
}

function DeleteFormatDialog({ slug, format, onClose }: { slug: string; format: NumberFormat | null; onClose: () => void }) {
  const del = useDeleteNumberFormat(slug)
  return (
    <ResponsiveDialog
      open={format !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Smazat číselnou řadu?"
      description={
        format && (
          <>
            Řada <code className="font-mono text-foreground">{format.format}</code> i její čítač budou odstraněny.
            Vystavené doklady si svá čísla ponechají.
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={() =>
              format &&
              del.mutate(format, {
                onSuccess: () => {
                  toast.success('Řada smazána')
                  onClose()
                },
                onError: (err) => {
                  toast.error(
                    isApiError(err) && err.status === 409
                      ? 'Výchozí ani poslední řadu dokladu nelze smazat. Nejdřív nastavte výchozí jinou řadu.'
                      : errorMessage(err),
                  )
                },
              })
            }
          >
            {del.isPending && <Spinner data-icon="inline-start" />}
            Smazat řadu
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
