import { useQuery } from '@tanstack/react-query'
import { TriangleAlertIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { adminQueries } from '@/api/queries/admin'
import type { InstanceStatus } from '@/api/types'
import { CopyButton } from '@/components/copy-button'
import { PageError } from '@/components/page-states'
import { FormSkeleton } from '@/components/skeletons'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDateTime } from '@/lib/date'
import { formatUptime } from '@/lib/mail-diag'

export function InstanceStatusPanel() {
  const status = useQuery(adminQueries.status())
  if (status.isPending) return <FormSkeleton fields={6} />
  if (status.isError) return <PageError error={status.error} reset={() => void status.refetch()} />
  return <StatusView st={status.data} />
}

const yes = (v: boolean, on = 'Ano', off = 'Ne') => (
  <Badge variant={v ? 'secondary' : 'outline'}>{v ? on : off}</Badge>
)

function StatusView({ st }: { st: InstanceStatus }) {
  const c = st.config
  return (
    <div className="flex flex-col gap-4">
      {(st.warnings ?? []).length > 0 && (
        <Alert>
          <TriangleAlertIcon className="text-warning" />
          <AlertTitle>Na co si dát pozor</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-1 pl-4">
              {(st.warnings ?? []).map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <InfoCard title="Instance">
          <Row label="Verze">{st.version}</Row>
          <Row label="Go">{st.go_version}</Row>
          <Row label="Databáze">{st.db_driver === 'postgres' ? 'PostgreSQL' : 'SQLite'}</Row>
          <Row label="Běží">
            {formatUptime(st.uptime_seconds)} <span className="text-muted-foreground">(od {formatDateTime(st.started_at)})</span>
          </Row>
          <Row label="Uživatelé">{st.users}</Row>
          <Row label="Účty (firmy)">{st.accounts}</Row>
        </InfoCard>

        <InfoCard title="Adresa a zabezpečení">
          <Row label="Veřejná adresa">
            <span className="break-all">{c.public_url}</span> {!c.public_https && <Badge variant="destructive">bez HTTPS</Badge>}
          </Row>
          <Row label="Omezení pokusů">{yes(c.rate_limit_enabled, 'Zapnuto', 'Vypnuto')}</Row>
          <Row label="Instalační token">{yes(c.setup_token_set, 'Nastaven', 'Nenastaven')}</Row>
          <Row label="Volná registrace">{yes(c.allow_signup, 'Povolena', 'Jen s pozvánkou')}</Row>
          <Row label="Důvěryhodné proxy">{(c.trusted_proxies ?? []).join(', ') || '—'}</Row>
          <Row label="Klíč pro tajné údaje">
            {c.secret_key_source === 'env' ? 'NANOFAKTURA_SECRET_KEY' : 'soubor secret.key v datovém adresáři'}
          </Row>
          <Row label="Datový adresář">
            <span className="break-all">{c.data_dir}</span>{' '}
            {!c.data_dir_writable && <Badge variant="destructive">nelze zapisovat</Badge>}
          </Row>
          <Row label="Dokumentace API">{yes(c.api_docs_enabled, 'Veřejná', 'Skrytá')}</Row>
          <Row label="Správci instance">{(c.admin_emails ?? []).join(', ') || '—'}</Row>
        </InfoCard>

        <InfoCard title="Odesílání e-mailů">
          {c.smtp_configured ? (
            <>
              <Row label="SMTP server">
                {c.smtp_host}:{c.smtp_port} <span className="text-muted-foreground">({c.smtp_tls})</span>
              </Row>
              <Row label="Přihlášení">{yes(c.smtp_auth, 'Jménem a heslem', 'Bez přihlášení')}</Row>
            </>
          ) : (
            <Row label="SMTP server">
              <Badge variant="destructive">Nenastaveno</Badge>{' '}
              <span className="text-muted-foreground">e-maily se jen vypisují do logu</span>
            </Row>
          )}
          <Row label="Odesílatel">{c.mail_from || '—'}</Row>
          <Row label="Podpis DKIM">
            {c.dkim_enabled ? (
              <>
                {yes(true, 'Zapnutý')} {c.dkim_selector}._domainkey.{c.dkim_domain} ({c.dkim_key_type})
              </>
            ) : (
              yes(false, '', 'Vypnutý')
            )}
          </Row>
        </InfoCard>

        {c.dkim_enabled && c.dkim_record && (
          <InfoCard title="DNS záznam pro DKIM">
            <p className="text-sm text-muted-foreground">
              TXT záznam na <code className="font-mono">{`${c.dkim_selector}._domainkey.${c.dkim_domain}`}</code>:
            </p>
            <div className="flex items-start gap-2">
              <code className="block min-w-0 flex-1 rounded-lg bg-muted px-3 py-2 font-mono text-xs break-all">{c.dkim_record}</code>
              <CopyButton value={c.dkim_record} />
            </div>
          </InfoCard>
        )}
      </div>
    </div>
  )
}

function InfoCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="flex flex-col gap-2.5 text-sm">{children}</dl>
      </CardContent>
    </Card>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-0.5 sm:grid-cols-[11rem_minmax(0,1fr)] sm:gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0">{children}</dd>
    </div>
  )
}
