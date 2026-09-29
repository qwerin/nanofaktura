import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { EyeIcon, LockIcon, RotateCcwIcon } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller, useForm, useWatch, type Path, type UseFormReturn } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { accountQueries, useUpdateAccount } from '@/api/queries/accounts'
import { invoiceQueries } from '@/api/queries/invoices'
import type { Account, EmailKind, EmailLang, UpdateAccountInput } from '@/api/types'
import {
  EMAIL_KINDS,
  EMAIL_LANGS,
  EMAIL_PLACEHOLDERS,
  emailKindDescriptions,
  emailKindLabels,
  emailVars,
  renderEmail,
  sampleEmailInvoice,
} from '@/components/email/placeholders'
import { ReminderDaysInput } from '@/components/email/reminder-days-input'
import { reminderSummary } from '@/components/email/reminders'
import { SwitchField, TextareaField, TextField } from '@/components/form/fields'
import { PageError } from '@/components/page-states'
import { PlaceholderChips } from '@/components/recurring/placeholder-chips'
import { Segmented } from '@/components/recurring/segmented'
import { usePlaceholderInsert } from '@/components/recurring/use-placeholder-insert'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { FormSkeleton } from '@/components/skeletons'
import { StickyActionBar } from '@/components/sticky-action-bar'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { todayISO } from '@/lib/date'
import { optionalEmailSchema } from '@/lib/validation'

export const Route = createFileRoute('/a/$slug/settings/emails')({
  loader: ({ context, params }) => context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'E-maily a upomínky · NanoFaktura' }] }),
  component: EmailsPage,
})

const templateSchema = z.object({
  kind: z.enum(['invoice', 'reminder', 'paid_thanks']),
  lang: z.enum(['cs', 'en']),
  subject: z.string().max(500, 'Nejvýše 500 znaků'),
  body: z.string().max(20000, 'Text je příliš dlouhý'),
  custom: z.boolean(),
})

const schema = z.object({
  email_reply_to: optionalEmailSchema,
  email_signature: z.string().max(2000, 'Nejvýše 2000 znaků'),
  reminders_enabled: z.boolean(),
  reminder_days_after_due: z.array(z.number()),
  paid_thanks_enabled: z.boolean(),
  templates: z.array(templateSchema),
})
type Values = z.infer<typeof schema>

function toValues(a: Account): Values {
  // Vždy všech 6 kombinací ve stálém pořadí (druh × jazyk).
  const templates = EMAIL_KINDS.flatMap((kind) =>
    EMAIL_LANGS.map((lang) => {
      const t = a.email_templates.find((x) => x.kind === kind && x.lang === lang)
      return { kind, lang, subject: t?.subject ?? '', body: t?.body ?? '', custom: t?.custom ?? false }
    }),
  )
  return {
    email_reply_to: a.email_reply_to,
    email_signature: a.email_signature,
    reminders_enabled: a.reminders_enabled,
    reminder_days_after_due: a.reminder_days_after_due,
    paid_thanks_enabled: a.paid_thanks_enabled,
    templates,
  }
}

function toPatch(v: Values, dirty: Partial<Record<keyof Values, unknown>>): UpdateAccountInput {
  const patch: UpdateAccountInput = {}
  if (dirty.email_reply_to) patch.email_reply_to = v.email_reply_to.trim()
  if (dirty.email_signature) patch.email_signature = v.email_signature
  if (dirty.reminders_enabled) patch.reminders_enabled = v.reminders_enabled
  if (dirty.reminder_days_after_due) patch.reminder_days_after_due = v.reminder_days_after_due
  if (dirty.paid_thanks_enabled) patch.paid_thanks_enabled = v.paid_thanks_enabled
  // PATCH šablon nahrazuje všechny přepisy → posíláme všechny; shodné s výchozím (nebo prázdné) backend neuloží.
  if (dirty.templates) {
    patch.email_templates = v.templates.map((t) => ({ kind: t.kind, lang: t.lang, subject: t.subject.trim(), body: t.body.trim() }))
  }
  return patch
}

function EmailsPage() {
  const { slug } = Route.useParams()
  const account = useQuery(accountQueries.detail(slug))
  return (
    <SettingsPage
      title="E-maily a upomínky"
      description="Jak vypadají e-maily s fakturami, komu klienti odpovídají a kdy se posílají automatické upomínky."
    >
      {account.isError ? (
        <PageError error={account.error} reset={() => void account.refetch()} />
      ) : account.data ? (
        <EmailsForm key={slug} slug={slug} account={account.data} />
      ) : (
        <FormSkeleton fields={6} className="max-w-2xl" />
      )}
    </SettingsPage>
  )
}

function EmailsForm({ slug, account }: { slug: string; account: Account }) {
  const { canManageSettings } = useCurrentAccount()
  const update = useUpdateAccount(slug)
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    values: toValues(account),
    resetOptions: { keepDirtyValues: true },
  })
  const { control, formState } = form
  const remindersEnabled = useWatch({ control, name: 'reminders_enabled' })
  const days = useWatch({ control, name: 'reminder_days_after_due' })
  const ids = { days: useId() }

  const onSubmit = form.handleSubmit(async (values) => {
    const patch = toPatch(values, formState.dirtyFields)
    if (Object.keys(patch).length === 0) return
    try {
      const saved = await update.mutateAsync(patch)
      form.reset(toValues(saved))
      toast.success('Nastavení e-mailů uloženo')
    } catch (err) {
      if (applyProblemToForm(err, form.setError)) toast.error('Zkontrolujte zvýrazněná pole.')
      else toast.error(errorMessage(err))
    }
  })

  const readOnly = !canManageSettings

  return (
    <form onSubmit={onSubmit} noValidate className="max-w-5xl">
      {readOnly && (
        <Alert className="mb-6">
          <LockIcon />
          <AlertDescription>Nastavení e-mailů může měnit jen vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}
      <fieldset disabled={readOnly || update.isPending} className="contents">
        <FormSection title="Odesílání" description="Odesílatele (From) určuje nastavení serveru; odpovědi klientů chodí na adresu níže.">
          <TextField
            control={control}
            name="email_reply_to"
            label="Odpovědi na adresu (Reply-To)"
            type="email"
            inputMode="email"
            autoComplete="email"
            autoCapitalize="none"
            spellCheck={false}
            placeholder={account.email || 'fakturace@firma.cz'}
            description={account.email ? `Prázdné = e-mail firmy (${account.email}).` : 'Prázdné = e-mail firmy z nastavení Firma.'}
          />
          <TextareaField
            control={control}
            name="email_signature"
            label="Podpis"
            rows={4}
            placeholder={`S pozdravem\n${account.name}`}
            description="Připojí se pod text každého e-mailu."
          />
        </FormSection>

        <FormSection title="Automatické upomínky" description="Plánovač pošle upomínku k neuhrazeným dokladům po splatnosti s e-mailem klienta.">
          <SwitchField control={control} name="reminders_enabled" label="Posílat upomínky automaticky" />
          <Controller
            control={control}
            name="reminder_days_after_due"
            render={({ field }) => (
              <Field data-disabled={!remindersEnabled || undefined}>
                <FieldLabel htmlFor={ids.days}>Kdy poslat</FieldLabel>
                <ReminderDaysInput id={ids.days} value={field.value} onChange={field.onChange} disabled={readOnly || !remindersEnabled} />
                <FieldDescription>
                  {remindersEnabled
                    ? `Upomínka ${reminderSummary(days)}. Když se nějaký krok zmešká, pošle se jen ten poslední dosažený.`
                    : 'Upomínky jsou vypnuté — ručně je pošlete z detailu faktury.'}
                </FieldDescription>
              </Field>
            )}
          />
          <SwitchField
            control={control}
            name="paid_thanks_enabled"
            label="Poděkovat za úhradu"
            description="Po zaplacení celé faktury odejde klientovi krátké poděkování (bez přílohy)."
          />
        </FormSection>

        <FormSection title="Texty e-mailů" description="Předvyplní se při odesílání a použijí se v automatických e-mailech. Jazyk podle jazyka dokladu.">
          <TemplateEditor form={form} account={account} slug={slug} readOnly={readOnly} />
        </FormSection>
      </fieldset>

      {!readOnly && (
        <StickyActionBar start={formState.isDirty ? 'Máte neuložené změny.' : 'Vše uloženo.'}>
          {formState.isDirty && (
            <Button type="button" variant="outline" onClick={() => form.reset()} disabled={update.isPending}>
              Zahodit
            </Button>
          )}
          <Button type="submit" disabled={!formState.isDirty || update.isPending}>
            {update.isPending && <Spinner data-icon="inline-start" />}
            Uložit změny
          </Button>
        </StickyActionBar>
      )}
    </form>
  )
}

function TemplateEditor({
  form,
  account,
  slug,
  readOnly,
}: {
  form: UseFormReturn<Values>
  account: Account
  slug: string
  readOnly: boolean
}) {
  const [kind, setKind] = useState<EmailKind>('invoice')
  const [lang, setLang] = useState<EmailLang>(account.default_language)
  const index = EMAIL_KINDS.indexOf(kind) * EMAIL_LANGS.length + EMAIL_LANGS.indexOf(lang)
  const { control } = form
  const templates = useWatch({ control, name: 'templates' })
  const signature = useWatch({ control, name: 'email_signature' })
  const t = templates[index]
  const ids = { subject: useId(), body: useId() }
  const dirty = form.formState.dirtyFields.templates?.[index]
  const reset = !t?.subject && !t?.body
  const placeholders = usePlaceholderInsert({
    accept: (name) => /^templates\.\d+\.(subject|body)$/.test(name),
    setValue: (name, value) => form.setValue(name as Path<Values>, value, { shouldDirty: true }),
    fallback: `templates.${index}.body`,
  })

  const revert = () => {
    form.setValue(`templates.${index}.subject`, '', { shouldDirty: true })
    form.setValue(`templates.${index}.body`, '', { shouldDirty: true })
    form.setValue(`templates.${index}.custom`, false, { shouldDirty: true })
  }

  return (
    <div className="flex min-w-0 flex-col gap-4" onFocusCapture={placeholders.onFocusCapture}>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="sm:flex-1">
          <Segmented
            value={kind}
            onChange={setKind}
            label="Druh e-mailu"
            options={EMAIL_KINDS.map((k) => ({ value: k, label: k === 'paid_thanks' ? 'Poděkování' : emailKindLabels[k] }))}
          />
        </div>
        <div className="sm:w-32">
          <Segmented value={lang} onChange={setLang} label="Jazyk" options={EMAIL_LANGS.map((l) => ({ value: l, label: l.toUpperCase() }))} />
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
        <span className="min-w-0 flex-1">{emailKindDescriptions[kind]}</span>
        {t?.custom || dirty ? <Badge variant="secondary">Vlastní text</Badge> : <Badge variant="outline">Výchozí text</Badge>}
        {!readOnly && (t?.custom || dirty) && !reset && (
          <Button type="button" variant="ghost" size="sm" onClick={revert} className="max-md:h-9">
            <RotateCcwIcon data-icon="inline-start" />
            Obnovit výchozí
          </Button>
        )}
      </div>

      <div className="grid min-w-0 gap-4 xl:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-4">
          <Field>
            <FieldLabel htmlFor={ids.subject}>Předmět</FieldLabel>
            <Input
              key={`s-${index}`}
              id={ids.subject}
              {...form.register(`templates.${index}.subject`)}
              placeholder={reset ? 'Výchozí předmět' : undefined}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={ids.body}>Text</FieldLabel>
            <Textarea
              key={`b-${index}`}
              id={ids.body}
              rows={12}
              {...form.register(`templates.${index}.body`)}
              placeholder={reset ? 'Výchozí text — doplní se po uložení.' : undefined}
              className="min-h-56 text-sm leading-relaxed"
            />
            {reset && <FieldDescription>Po uložení se použije výchozí text aplikace.</FieldDescription>}
          </Field>
          {!readOnly && (
            <div className="flex flex-col gap-2">
              <p className="text-xs font-medium text-muted-foreground">Vložit na místo kurzoru</p>
              <PlaceholderChips items={EMAIL_PLACEHOLDERS} onInsert={placeholders.insert} />
            </div>
          )}
        </div>
        <EmailPreviewCard slug={slug} account={account} lang={lang} subject={t?.subject ?? ''} body={t?.body ?? ''} signature={signature} />
      </div>
    </div>
  )
}

/** Živý náhled rozepsaného textu s daty poslední faktury (nebo ukázkovými). */
function EmailPreviewCard({
  slug,
  account,
  lang,
  subject,
  body,
  signature,
}: {
  slug: string
  account: Account
  lang: EmailLang
  subject: string
  body: string
  signature: string
}) {
  const recent = useQuery(invoiceQueries.top(slug, {}, 1))
  const [today] = useState(todayISO)
  const inv = recent.data?.items[0]
  const vars = emailVars(inv ?? sampleEmailInvoice(today, account.default_currency || 'CZK'), {
    accountName: account.name,
    lang,
    origin: window.location.origin,
    today,
  })
  const out = renderEmail({ subject, body }, vars, signature)
  const empty = !subject && !body

  return (
    <section aria-label="Náhled e-mailu" className="flex min-w-0 flex-col overflow-hidden rounded-xl border bg-muted/30">
      <div className="flex items-center gap-2 border-b px-4 py-2.5 text-xs text-muted-foreground">
        <EyeIcon className="size-3.5" />
        <span className="min-w-0 flex-1 truncate">
          Náhled {inv ? `s fakturou ${inv.number} · ${inv.client_name}` : 's ukázkovými daty'}
        </span>
      </div>
      {empty ? (
        <p className="p-4 text-sm text-muted-foreground">Náhled výchozího textu uvidíte po uložení.</p>
      ) : (
        <div className="flex min-w-0 flex-col gap-3 p-4">
          <p className="text-sm font-semibold break-words">{out.subject || '(bez předmětu)'}</p>
          <p className="text-sm break-words whitespace-pre-line">{out.body}</p>
        </div>
      )}
    </section>
  )
}

