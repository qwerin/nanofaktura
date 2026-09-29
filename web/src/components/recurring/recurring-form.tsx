// Formulář pravidelné faktury: šablona + rozvrh (začátek, perioda, den v měsíci, konec) + odeslání e-mailem.

import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Link, useBlocker } from '@tanstack/react-router'
import { CalendarClockIcon, FilePlus2Icon, MailWarningIcon, TriangleAlertIcon } from 'lucide-react'
import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateRecurring, useUpdateRecurring } from '@/api/queries/recurring'
import { subjectQueries } from '@/api/queries/subjects'
import { templateQueries } from '@/api/queries/templates'
import type { Account, Recurring } from '@/api/types'
import { SwitchField, TextField } from '@/components/form/fields'
import { documentTypeShortLabels } from '@/components/invoice/status'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { formatDate, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import { FormCard } from './form-card'
import { PERIOD_PRESETS, periodChipLabel, periodLabel, upcomingOccurrences } from './period'
import {
  missedOccurrences,
  newRecurringValues,
  recurringFormSchema,
  recurringToValues,
  toCreateRecurringBody,
  toPatchRecurringBody,
  type RecurringFormValues,
} from './recurring-model'
import { Segmented } from './segmented'
import { templateTotal } from './template-model'

interface RecurringFormProps {
  slug: string
  account: Account
  recurring?: Recurring
  presetTemplateId?: number
  onSaved: (r: Recurring) => void
}

export function RecurringForm({ slug, account, recurring, presetTemplateId, onSaved }: RecurringFormProps) {
  const isEdit = Boolean(recurring)
  const create = useCreateRecurring(slug)
  const update = useUpdateRecurring(slug, recurring?.id ?? 0)
  const pending = create.isPending || update.isPending
  const allowLeave = useRef(false)
  const templates = useQuery(templateQueries.list(slug))
  const [today] = useState(todayISO)

  const form = useForm<RecurringFormValues>({
    resolver: zodResolver(recurringFormSchema),
    defaultValues: recurring ? recurringToValues(recurring) : newRecurringValues(today, presetTemplateId),
  })
  const { control, formState, setValue } = form
  const [templateId, startOn, monthsPeriod, dayOfMonth, endOn, sendEmail] = useWatch({
    control,
    name: ['template_id', 'start_on', 'months_period', 'day_of_month', 'end_on', 'send_email'],
  })

  const templateItems = (templates.data?.items ?? []).map((t) => ({ value: String(t.id), label: t.name }))
  const template = templates.data?.items.find((t) => String(t.id) === templateId)
  const subject = useQuery({ ...subjectQueries.detail(slug, template?.subject_id ?? 0), enabled: Boolean(template?.subject_id) })

  // Název doplníme ze šablony, dokud ho uživatel nezměnil.
  const autoName = useRef('')
  const templateName = template?.name
  useEffect(() => {
    if (isEdit || !templateName) return
    const current = form.getValues('name')
    if (current === '' || current === autoName.current) {
      autoName.current = templateName
      setValue('name', templateName, { shouldDirty: true })
    }
  }, [templateName, isEdit, form, setValue])

  const period = Number(monthsPeriod) || 0
  const custom = !PERIOD_PRESETS.includes(period as (typeof PERIOD_PRESETS)[number])
  const [customOpen, setCustomOpen] = useState(custom)
  const schedule = { start: startOn, monthsPeriod: period, dayOfMonth: Number(dayOfMonth) || 0, endOn: endOn || undefined }
  const upcoming = period >= 1 ? upcomingOccurrences(schedule, 4) : []
  const missed = !isEdit && period >= 1 ? missedOccurrences(schedule, today) : 0
  const scheduleChanged = isEdit && (formState.dirtyFields.start_on || formState.dirtyFields.day_of_month)

  const blocker = useBlocker({
    shouldBlockFn: () => formState.isDirty && !allowLeave.current,
    enableBeforeUnload: () => formState.isDirty && !allowLeave.current,
    withResolver: true,
  })

  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    void form.handleSubmit(
    async (values) => {
      try {
        let saved: Recurring
        if (recurring) {
          const patch = toPatchRecurringBody(values, formState.dirtyFields)
          saved = Object.keys(patch).length ? await update.mutateAsync(patch) : recurring
        } else {
          saved = await create.mutateAsync(toCreateRecurringBody(values))
        }
        toast.success(isEdit ? 'Změny uloženy' : `Pravidelná faktura „${saved.name}“ založena`)
        allowLeave.current = true
        onSaved(saved)
      } catch (err) {
        if (applyProblemToForm(err, form.setError)) toast.error('Zkontrolujte zvýrazněná pole.')
        else toast.error(errorMessage(err))
      }
    },
    () => toast.error('Zkontrolujte zvýrazněná pole.'),
    )()
  }

  const ids = { template: useId(), period: useId(), day: useId() }

  return (
    <form noValidate onSubmit={onSubmit} className="flex max-w-3xl flex-col gap-4 lg:gap-6">
      <FormCard title="Co vystavovat">
        <Controller
          control={control}
          name="template_id"
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid || undefined}>
              <FieldLabel htmlFor={ids.template}>Šablona faktury</FieldLabel>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Select items={templateItems} value={field.value || null} onValueChange={(v) => field.onChange(v ?? '')}>
                  <SelectTrigger id={ids.template} ref={field.ref} className="w-full sm:flex-1" aria-invalid={fieldState.invalid || undefined}>
                    <SelectValue placeholder={templates.isPending ? 'Načítám…' : 'Vyberte šablonu…'} />
                  </SelectTrigger>
                  <SelectContent>
                    {templateItems.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  nativeButton={false}
                  render={<Link to="/a/$slug/templates/new" params={{ slug }} search={{ next: 'recurring' }} />}
                >
                  <FilePlus2Icon data-icon="inline-start" />
                  Nová šablona
                </Button>
              </div>
              {template && (
                <FieldDescription>
                  {subject.data?.name ?? '…'} · {template.lines.length}{' '}
                  {template.lines.length === 1 ? 'položka' : template.lines.length < 5 ? 'položky' : 'položek'} ·{' '}
                  <span className="font-medium text-foreground tabular-nums">
                    {formatMoney(templateTotal(template, account), template.currency || account.default_currency)}
                  </span>{' '}
                  ·{' '}
                  <Link
                    to="/a/$slug/templates/$templateId"
                    params={{ slug, templateId: template.id }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    upravit šablonu
                  </Link>
                </FieldDescription>
              )}
              {!templates.isPending && templateItems.length === 0 && (
                <FieldDescription>Zatím nemáte žádnou šablonu — založte ji tlačítkem „Nová šablona“.</FieldDescription>
              )}
              <FieldError errors={[fieldState.error]} />
            </Field>
          )}
        />
        <TextField control={control} name="name" label="Název" autoComplete="off" description="Pro vaši orientaci v seznamu." />
        <Controller
          control={control}
          name="issue_as"
          render={({ field }) => (
            <Field>
              <FieldLabel>Vystavit jako</FieldLabel>
              <Segmented
                value={field.value}
                onChange={field.onChange}
                label="Vystavit jako"
                options={(['invoice', 'proforma'] as const).map((t) => ({ value: t, label: documentTypeShortLabels[t] }))}
              />
            </Field>
          )}
        />
      </FormCard>

      <FormCard title="Kdy vystavovat">
        <Controller
          control={control}
          name="months_period"
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid || undefined}>
              <FieldLabel htmlFor={ids.period}>Opakování</FieldLabel>
              <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-5">
                {PERIOD_PRESETS.map((m) => (
                  <Button
                    key={m}
                    type="button"
                    variant={!customOpen && field.value === String(m) ? 'secondary' : 'outline'}
                    className={cn(!customOpen && field.value === String(m) && 'ring-1 ring-primary/40')}
                    aria-pressed={!customOpen && field.value === String(m)}
                    onClick={() => {
                      setCustomOpen(false)
                      field.onChange(String(m))
                    }}
                  >
                    {periodChipLabel(m)}
                  </Button>
                ))}
                <Button
                  type="button"
                  variant={customOpen ? 'secondary' : 'outline'}
                  className={cn('col-span-2 sm:col-span-1', customOpen && 'ring-1 ring-primary/40')}
                  aria-pressed={customOpen}
                  onClick={() => setCustomOpen(true)}
                >
                  Jiné…
                </Button>
              </div>
              {customOpen && (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-muted-foreground">Každých</span>
                  <Input
                    id={ids.period}
                    {...field}
                    inputMode="numeric"
                    autoComplete="off"
                    maxLength={3}
                    className="w-20"
                    aria-invalid={fieldState.invalid || undefined}
                  />
                  <span className="text-sm text-muted-foreground">měsíců</span>
                </div>
              )}
              {period >= 1 && <FieldDescription>Faktura se vystaví {periodLabel(period)}.</FieldDescription>}
              <FieldError errors={[fieldState.error]} />
            </Field>
          )}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextField control={control} name="start_on" label="Začátek" type="date" />
          <Controller
            control={control}
            name="day_of_month"
            render={({ field, fieldState }) => (
              <Field data-invalid={fieldState.invalid || undefined}>
                <FieldLabel htmlFor={ids.day}>Den v měsíci</FieldLabel>
                <div className="flex gap-2">
                  <Input
                    id={ids.day}
                    {...field}
                    inputMode="numeric"
                    autoComplete="off"
                    maxLength={2}
                    placeholder="—"
                    className="w-20"
                    aria-invalid={fieldState.invalid || undefined}
                  />
                  <Button
                    type="button"
                    variant={field.value === '1' ? 'secondary' : 'ghost'}
                    className="flex-1 px-2"
                    onClick={() => field.onChange(field.value === '1' ? '' : '1')}
                  >
                    1.
                  </Button>
                  <Button
                    type="button"
                    variant={field.value === '31' ? 'secondary' : 'ghost'}
                    className="flex-1 px-2"
                    onClick={() => field.onChange(field.value === '31' ? '' : '31')}
                  >
                    Poslední
                  </Button>
                </div>
                <FieldDescription>
                  {field.value === '31'
                    ? 'Poslední den měsíce.'
                    : field.value
                      ? 'V kratších měsících poslední den.'
                      : 'Prázdné = stejný den jako začátek.'}
                </FieldDescription>
                <FieldError errors={[fieldState.error]} />
              </Field>
            )}
          />
          <TextField control={control} name="end_on" label="Konec (nepovinné)" type="date" description="Poslední možné datum vystavení." />
        </div>

        {isEdit && recurring ? (
          <ScheduleBox>
            {scheduleChanged ? (
              <>Příští datum vystavení se po uložení přepočítá podle nového začátku a dne v měsíci.</>
            ) : (
              <>
                Příští vystavení <span className="font-medium text-foreground">{formatDate(recurring.next_occurrence_on)}</span>
              </>
            )}
          </ScheduleBox>
        ) : (
          upcoming.length > 0 && (
            <ScheduleBox>
              <span>
                Vystaví se{' '}
                {upcoming.map((d, i) => (
                  <span key={d}>
                    {i > 0 && ', '}
                    <span className="font-medium whitespace-nowrap text-foreground tabular-nums">{formatDate(d)}</span>
                  </span>
                ))}
                {upcoming.length === 4 && ' …'}
              </span>
            </ScheduleBox>
          )
        )}
        {missed > 1 && (
          <Alert>
            <TriangleAlertIcon />
            <AlertDescription>
              Začátek je v minulosti — po uložení se zpětně vystaví {missed} faktur, každá k datu svého výskytu. Pokud to nechcete,
              posuňte začátek na dnešek nebo později.
            </AlertDescription>
          </Alert>
        )}
        {missed === 1 && <FieldDescription>První faktura se vystaví hned po uložení (plánovač běží každou hodinu).</FieldDescription>}
      </FormCard>

      <FormCard title="Odeslání">
        <SwitchField
          control={control}
          name="send_email"
          label="Posílat klientovi e-mailem"
          description="Každá vystavená faktura odejde s PDF na e-mail odběratele (a kopie dle kontaktu)."
        />
        {sendEmail && template && subject.data && !subject.data.email && (
          <Alert>
            <MailWarningIcon />
            <AlertDescription>
              <span>
                Kontakt {subject.data.name} nemá vyplněný e-mail — e-maily se nepodaří odeslat (chyba se zapíše do historie faktury).{' '}
                <Link
                  to="/a/$slug/subjects/$subjectId/edit"
                  params={{ slug, subjectId: subject.data.id }}
                  className="font-medium text-foreground underline underline-offset-4"
                >
                  Doplnit e-mail
                </Link>
              </span>
            </AlertDescription>
          </Alert>
        )}
        {!isEdit && (
          <SwitchField control={control} name="active" label="Aktivní" description="Vypnutá pravidelná faktura nic nevystavuje." />
        )}
      </FormCard>

      <StickyActionBar start={formState.isDirty ? 'Máte neuložené změny.' : undefined}>
        <Button type="submit" disabled={pending || (isEdit && !formState.isDirty)}>
          {pending && <Spinner data-icon="inline-start" />}
          {isEdit ? 'Uložit změny' : 'Založit'}
        </Button>
      </StickyActionBar>

      <ResponsiveDialog
        open={blocker.status === 'blocked'}
        onOpenChange={(o) => !o && blocker.reset?.()}
        title="Zahodit neuložené změny?"
        description="Pokud odejdete, změny se ztratí."
        footer={
          <>
            <Button variant="destructive" onClick={() => blocker.proceed?.()}>
              Zahodit a odejít
            </Button>
            <Button variant="outline" onClick={() => blocker.reset?.()}>
              Zůstat
            </Button>
          </>
        }
      />
    </form>
  )
}

function ScheduleBox({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-start gap-2 rounded-lg bg-muted/60 px-3 py-2.5 text-sm text-muted-foreground">
      <CalendarClockIcon className="mt-0.5 size-4 shrink-0 text-primary" />
      {children}
    </div>
  )
}

