import { zodResolver } from '@hookform/resolvers/zod'
import { CircleCheckIcon, CircleXIcon, TriangleAlertIcon } from 'lucide-react'
import { useEffect } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateWebhook, useDeleteWebhook, useUpdateWebhook } from '@/api/queries/webhooks'
import type { Webhook, WebhookTestResult } from '@/api/types'
import { CopyButton } from '@/components/copy-button'
import { SwitchField, TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import { EventPicker } from './event-picker'
import { ALL_EVENTS } from './webhook-events'

const formSchema = z.object({
  url: z
    .string()
    .trim()
    .min(1, 'Zadejte adresu')
    .max(2000, 'Příliš dlouhá adresa')
    .refine((v) => /^https?:\/\/\S+$/i.test(v), 'Adresa musí začínat http:// nebo https://'),
  description: z.string().trim().max(200, 'Nejvýše 200 znaků'),
  events: z.array(z.string()).min(1, 'Vyberte alespoň jednu událost'),
  active: z.boolean(),
})
type FormValues = z.infer<typeof formSchema>

/** Vytvoření / úprava webhooku. Po vytvoření předá rodiči webhook s jednorázově viditelným tajemstvím. */
export function WebhookFormDialog({
  slug,
  open,
  webhook,
  onClose,
  onCreated,
}: {
  slug: string
  open: boolean
  /** `null` = nový webhook. */
  webhook: Webhook | null
  onClose: () => void
  onCreated: (w: Webhook) => void
}) {
  const create = useCreateWebhook(slug)
  const update = useUpdateWebhook(slug)
  const pending = create.isPending || update.isPending
  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: { url: '', description: '', events: [ALL_EVENTS], active: true },
  })

  useEffect(() => {
    if (!open) return
    form.reset(
      webhook
        ? { url: webhook.url, description: webhook.description, events: webhook.events.length ? webhook.events : [ALL_EVENTS], active: webhook.active }
        : { url: '', description: '', events: [ALL_EVENTS], active: true },
    )
  }, [open, webhook, form])

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      if (webhook) {
        await update.mutateAsync({ id: webhook.id, body: values })
        toast.success('Webhook uložen')
        onClose()
      } else {
        const created = await create.mutateAsync(values)
        onCreated(created)
      }
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={webhook ? 'Upravit webhook' : 'Nový webhook'}
      description="Na adresu pošleme POST s JSONem při každé vybrané události."
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="submit" form="webhook-form" disabled={pending}>
            {pending && <Spinner data-icon="inline-start" />}
            {webhook ? 'Uložit' : 'Vytvořit webhook'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id="webhook-form" onSubmit={onSubmit} noValidate className="-mx-1 flex flex-col gap-5 px-1 md:max-h-[62vh] md:overflow-y-auto">
        <TextField
          control={form.control}
          name="url"
          label="URL"
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://example.com/hooks/nanofaktura"
        />
        <TextField control={form.control} name="description" label="Popis" placeholder="např. Synchronizace do CRM" autoComplete="off" />
        <Controller
          control={form.control}
          name="events"
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid || undefined}>
              <FieldLabel>Události</FieldLabel>
              <EventPicker slug={slug} value={field.value} onChange={field.onChange} />
              <FieldError errors={[fieldState.error]} />
            </Field>
          )}
        />
        <SwitchField
          control={form.control}
          name="active"
          label="Aktivní"
          description={webhook?.disabled_at ? 'Zapnutím vynulujete počítadlo neúspěšných doručení.' : 'Neaktivní webhook nic nedostává (test funguje).'}
        />
      </form>
    </ResponsiveDialog>
  )
}

/** Tajemství pro podpis — zobrazí se jen jednou (po vytvoření / otočení). */
export function SecretDialog({ webhook, onClose }: { webhook: Webhook | null; onClose: () => void }) {
  const secret = webhook?.secret ?? ''
  return (
    <ResponsiveDialog
      open={webhook !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Tajemství pro ověření podpisu"
      description={webhook?.url}
      footer={<Button onClick={onClose}>Hotovo, mám ho uložené</Button>}
    >
      <div className="flex flex-col gap-4">
        <Alert>
          <TriangleAlertIcon />
          <AlertTitle>Zkopírujte si ho hned</AlertTitle>
          <AlertDescription>Z bezpečnostních důvodů ho už znovu nezobrazíme. Když ho ztratíte, vygenerujte nové.</AlertDescription>
        </Alert>
        <code className="block rounded-lg border bg-muted px-3 py-3 font-mono text-sm break-all select-all" aria-label="Tajemství webhooku">
          {secret}
        </code>
        <CopyButton value={secret} label="Zkopírovat tajemství" className="w-full" />
      </div>
    </ResponsiveDialog>
  )
}

/** Výsledek „Poslat test“ (ping). */
export function TestResultDialog({ result, onClose }: { result: WebhookTestResult | null; onClose: () => void }) {
  const ok = result?.ok
  return (
    <ResponsiveDialog
      open={result !== null}
      onOpenChange={(o) => !o && onClose()}
      title={ok ? 'Test proběhl úspěšně' : 'Test se nezdařil'}
      footer={<Button onClick={onClose}>Zavřít</Button>}
    >
      {result && (
        <div className="flex flex-col gap-4 text-sm">
          <div className="flex items-center gap-3">
            {ok ? <CircleCheckIcon className="size-8 text-success" /> : <CircleXIcon className="size-8 text-destructive" />}
            <div className="flex flex-col">
              <span className="font-medium">{result.status_code ? `HTTP ${result.status_code}` : 'Bez odpovědi'}</span>
              <span className="text-muted-foreground">za {result.duration_ms} ms</span>
            </div>
          </div>
          {result.error && <p className="rounded-lg bg-destructive/10 px-3 py-2 break-words text-destructive">{result.error}</p>}
          <ResponseSnippet body={result.response_body} />
        </div>
      )}
    </ResponsiveDialog>
  )
}

export function ResponseSnippet({ body, label = 'Odpověď (prvních 1 kB)', className }: { body: string; label?: string; className?: string }) {
  if (!body) return null
  return (
    <div className={cn('flex flex-col gap-1', className)}>
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <pre className="max-h-48 overflow-auto rounded-lg border bg-muted px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all">{body}</pre>
    </div>
  )
}

export function DeleteWebhookDialog({ slug, webhook, onClose }: { slug: string; webhook: Webhook | null; onClose: () => void }) {
  const del = useDeleteWebhook(slug)
  return (
    <ResponsiveDialog
      open={webhook !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Smazat webhook?"
      description={
        webhook && (
          <>
            Na <strong className="break-all text-foreground">{webhook.url}</strong> už nic nepošleme. Akci nelze vrátit.
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={() =>
              webhook &&
              del.mutate(webhook, {
                onSuccess: () => {
                  toast.success('Webhook smazán')
                  onClose()
                },
              })
            }
          >
            {del.isPending && <Spinner data-icon="inline-start" />}
            Smazat
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
