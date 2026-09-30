import { ChevronDownIcon, MailCheckIcon, SendIcon } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useEmailTest } from '@/api/queries/admin'
import type { EmailTestCheck, EmailTestReport } from '@/api/types'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { formatDuration, groupChecks, reportSummary, statusLabels } from '@/lib/mail-diag'
import { cn } from '@/lib/utils'
import { StatusIcon } from './status-icon'

const fieldMessages: Record<string, string> = {
  to: 'Zadejte platnou e-mailovou adresu.',
  dkim_selector: 'Selektor smí obsahovat jen písmena, číslice, tečku, pomlčku a podtržítko.',
}

/** Formulář testu e-mailu + výsledek po krocích. */
export function EmailTestPanel({ defaultTo }: { defaultTo: string }) {
  const test = useEmailTest()
  const [to, setTo] = useState(defaultTo)
  const [selector, setSelector] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setErrors({})
    test.mutate(
      { to: to.trim() || undefined, dkim_selector: selector.trim() || undefined },
      {
        onError: (err) => {
          const found: Record<string, string> = {}
          applyProblemToForm(err, (name) => {
            found[String(name)] = fieldMessages[String(name)] ?? 'Neplatná hodnota'
          })
          setErrors(found)
        },
      },
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Odeslat testovací e-mail</CardTitle>
          <p className="text-sm text-muted-foreground">
            Projde spojení se SMTP serverem krok po kroku, zkontroluje DNS záznamy domény odesílatele (SPF, DKIM, DMARC, MX)
            a pošle skutečný e-mail. Trvá to obvykle pár sekund.
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} noValidate>
            <FieldGroup className="gap-4">
              <div className="grid gap-4 md:grid-cols-2">
                <Field data-invalid={errors.to ? true : undefined}>
                  <FieldLabel htmlFor="email-test-to">Příjemce</FieldLabel>
                  <Input
                    id="email-test-to"
                    type="email"
                    inputMode="email"
                    autoComplete="email"
                    value={to}
                    onChange={(e) => setTo(e.target.value)}
                    aria-invalid={errors.to ? true : undefined}
                  />
                  <FieldDescription>Pro kontrolu podpisů pošlete test na Gmail — umí zobrazit výsledek SPF/DKIM/DMARC.</FieldDescription>
                  {errors.to && <FieldError>{errors.to}</FieldError>}
                </Field>
                <Field data-invalid={errors.dkim_selector ? true : undefined}>
                  <FieldLabel htmlFor="email-test-selector">Selektor DKIM (nepovinné)</FieldLabel>
                  <Input
                    id="email-test-selector"
                    value={selector}
                    onChange={(e) => setSelector(e.target.value)}
                    placeholder="např. google, selector1, default"
                    autoCapitalize="off"
                    autoCorrect="off"
                    spellCheck={false}
                    aria-invalid={errors.dkim_selector ? true : undefined}
                  />
                  <FieldDescription>
                    Když e-maily podepisuje váš poskytovatel SMTP, zadejte jeho selektor a ověříme jeho DNS záznam.
                  </FieldDescription>
                  {errors.dkim_selector && <FieldError>{errors.dkim_selector}</FieldError>}
                </Field>
              </div>
              <div>
                <Button type="submit" disabled={test.isPending} className="w-full sm:w-auto">
                  {test.isPending ? <Spinner data-icon="inline-start" /> : <SendIcon data-icon="inline-start" />}
                  {test.isPending ? 'Testuji…' : 'Spustit test'}
                </Button>
              </div>
              {test.isError && Object.keys(errors).length === 0 && (
                <p className="text-sm text-destructive">{errorMessage(test.error)}</p>
              )}
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      {test.data && <EmailTestResult report={test.data} />}
    </div>
  )
}

export function EmailTestResult({ report }: { report: EmailTestReport }) {
  const groups = groupChecks(report)
  return (
    <section aria-label="Výsledek testu" className="flex flex-col gap-4" data-testid="email-test-result">
      <Alert variant={report.sent ? 'default' : 'destructive'}>
        {report.sent ? <MailCheckIcon className="text-success" /> : <StatusIcon status="error" />}
        <AlertTitle>{report.sent ? 'E-mail odeslán' : 'E-mail se nepodařilo odeslat'}</AlertTitle>
        <AlertDescription>
          <p>{reportSummary(report)}</p>
          <p className="text-xs">
            {report.from} → {report.to}
            {report.smtp_host && ` · ${report.smtp_host}:${report.smtp_port} (${report.tls_mode})`}
            {report.queue_id && ` · ID ve frontě ${report.queue_id}`}
            {` · ${formatDuration(report.duration_ms)}`}
            {report.dkim_signed && ' · podepsáno DKIM'}
          </p>
        </AlertDescription>
      </Alert>

      {groups.map((g) => (
        <Card key={g.id} size="sm" className="gap-0 py-0">
          <div className="flex items-center gap-2 border-b px-4 py-3">
            <StatusIcon status={g.status} />
            <h3 className="flex-1 text-sm font-semibold">{g.label}</h3>
            <span className="text-xs text-muted-foreground">{statusLabels[g.status]}</span>
          </div>
          <ul className="divide-y">
            {g.checks.map((c) => (
              <CheckRow key={c.id} check={c} />
            ))}
          </ul>
        </Card>
      ))}

      {report.sent && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Jak ověřit doručení</CardTitle>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">
            <ol className="list-decimal space-y-1 pl-5">
              <li>Otevřete testovací e-mail ve schránce příjemce (zkontrolujte i spam).</li>
              <li>V Gmailu klikněte na tři tečky vpravo nahoře a zvolte „Zobrazit originál“.</li>
              <li>
                U SPF, DKIM i DMARC by mělo být „PASS“ (v hlavičce Authentication-Results <code>spf=pass</code>,{' '}
                <code>dkim=pass</code>, <code>dmarc=pass</code>).
              </li>
            </ol>
          </CardContent>
        </Card>
      )}
    </section>
  )
}

function CheckRow({ check }: { check: EmailTestCheck }) {
  const details = check.details ?? []
  const hasMore = details.length > 0
  const body = (
    <>
      <StatusIcon status={check.status} className="mt-0.5" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="min-w-0 flex-1 text-sm font-medium">{check.title}</span>
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{formatDuration(check.duration_ms)}</span>
        </div>
        <p className="mt-0.5 text-sm break-words text-muted-foreground">{check.message}</p>
        {check.hint && (
          <p
            className={cn(
              'mt-1.5 rounded-md px-2.5 py-1.5 text-sm break-words',
              check.status === 'error' ? 'bg-destructive/10 text-destructive' : 'bg-muted text-foreground',
            )}
          >
            {check.hint}
          </p>
        )}
      </div>
      {hasMore && (
        <ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
      )}
    </>
  )
  if (!hasMore) return <li className="flex gap-3 px-4 py-3">{body}</li>
  return (
    <li>
      <details className="group [&_summary::-webkit-details-marker]:hidden">
        <summary className="flex cursor-pointer list-none gap-3 px-4 py-3 hover:bg-muted/50" aria-label={`${check.title} — podrobnosti`}>
          {body}
        </summary>
        <pre className="mx-4 mb-3 max-h-72 overflow-auto rounded-lg bg-muted px-3 py-2 font-mono text-xs whitespace-pre-wrap break-all">
          {details.join('\n')}
        </pre>
      </details>
    </li>
  )
}
