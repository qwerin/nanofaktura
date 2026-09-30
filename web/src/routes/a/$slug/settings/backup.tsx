import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ArchiveRestoreIcon, CheckIcon, DownloadIcon, ShieldOffIcon, TerminalIcon, TriangleAlertIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { api, unwrap } from '@/api/client'
import { errorMessage } from '@/api/errors'
import { downloadBackup } from '@/api/queries/backup'
import { keys } from '@/api/queries/keys'
import { FormSection, SettingsPage } from '@/components/settings-page'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatBytes } from '@/lib/backup'
import { formatDateTime } from '@/lib/date'

export const Route = createFileRoute('/a/$slug/settings/backup')({
  head: () => ({ meta: [{ title: 'Záloha a přenos · NanoFaktura' }] }),
  component: BackupPage,
})

const included = [
  'Firma, všechna nastavení, logo a podpis',
  'Bankovní účty a číselné řady včetně čítačů (další číslo navazuje)',
  'Kontakty, ceník a pohyby skladu',
  'Faktury, zálohy a dobropisy s řádky a platbami',
  'Náklady s řádky a platbami',
  'Šablony a pravidelné faktury',
  'Bankovní pohyby a jejich párování',
  'Úkoly, historie událostí a odeslané e-maily',
  'Webhooky a všechny přílohy (soubory)',
]

const excluded = [
  'Tokeny Fio API (zadáte znovu u bankovního účtu)',
  'Tajemství webhooků (po obnově se vygeneruje nové)',
  'Hesla, přihlášení, API tokeny a pozvánky',
  'Ostatní členové účtu — záloha je jen vypíše, po obnově je pozvěte znovu',
]

function BackupPage() {
  const { slug } = Route.useParams()
  const { canManageSettings } = useCurrentAccount()
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  const last = useQuery({
    queryKey: [...keys.eventList(slug, { name: 'account.exported', perPage: 1 })],
    queryFn: ({ signal }) =>
      unwrap(
        api.GET('/api/accounts/{slug}/events', {
          params: { path: { slug }, query: { name: 'account.exported', per_page: 1 } },
          signal,
        }),
      ),
    enabled: canManageSettings,
  })
  const lastExport = last.data?.items?.[0]

  const onDownload = async () => {
    setBusy(true)
    const id = toast.loading('Připravuji zálohu…')
    try {
      const { name, size } = await downloadBackup(slug, (loaded) =>
        toast.loading(`Stahuji zálohu… ${formatBytes(loaded)}`, { id }),
      )
      toast.success(`Záloha stažena (${formatBytes(size)})`, { id, description: name })
      await qc.invalidateQueries({ queryKey: keys.events(slug) })
    } catch (err) {
      toast.error(errorMessage(err), { id })
    } finally {
      setBusy(false)
    }
  }

  return (
    <SettingsPage
      title="Záloha a přenos"
      description="Kompletní záloha účtu v jednom ZIP souboru. Poslouží jako archiv i pro přenos účtu na jinou instanci NanoFaktury (např. ze SQLite na PostgreSQL)."
    >
      {!canManageSettings ? (
        <Alert>
          <TriangleAlertIcon />
          <AlertDescription>Zálohu účtu může stáhnout jen vlastník nebo administrátor.</AlertDescription>
        </Alert>
      ) : (
        <>
          <FormSection
            title="Stáhnout zálohu"
            description="Soubor obsahuje citlivé údaje o firmě a klientech — uložte ho na bezpečné místo."
          >
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
              <Button onClick={() => void onDownload()} disabled={busy} className="w-full sm:w-auto">
                {busy ? <Spinner data-icon="inline-start" /> : <DownloadIcon data-icon="inline-start" />}
                Stáhnout zálohu
              </Button>
              <p className="text-sm text-muted-foreground">
                {lastExport
                  ? `Poslední záloha: ${formatDateTime(lastExport.created_at)}${lastExport.user_name ? ` (${lastExport.user_name})` : ''}`
                  : last.isSuccess
                    ? 'Záloha zatím nebyla stažena.'
                    : null}
              </p>
            </div>
          </FormSection>

          <FormSection title="Co záloha obsahuje">
            <ul className="flex flex-col gap-2 text-sm">
              {included.map((t) => (
                <li key={t} className="flex gap-2">
                  <CheckIcon className="mt-0.5 size-4 shrink-0 text-success" />
                  {t}
                </li>
              ))}
            </ul>
          </FormSection>

          <FormSection title="Co záloha neobsahuje" description="Tajné údaje se nikdy neexportují.">
            <ul className="flex flex-col gap-2 text-sm">
              {excluded.map((t) => (
                <li key={t} className="flex gap-2">
                  <ShieldOffIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                  {t}
                </li>
              ))}
            </ul>
          </FormSection>

          <FormSection
            title="Obnovit ze zálohy"
            description="Obnova vždy založí nový účet — stávající data se nepřepisují."
          >
            <div className="flex gap-3 text-sm">
              <ArchiveRestoreIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <p>
                V přepínači účtů zvolte <strong>Nový účet → Obnovit ze zálohy</strong> a nahrajte ZIP. Pravidelné faktury
                se obnoví vypnuté (aby je dvě instance nevystavily dvakrát), upomínky a poděkování za platbu také — zapnete
                je v nastavení.
              </p>
            </div>
            <div className="flex gap-3 text-sm">
              <TerminalIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <p>Správce serveru může zálohovat i obnovovat z příkazové řádky:</p>
                <pre className="mt-2 overflow-x-auto rounded-lg bg-muted p-3 text-xs">
                  {`nanofaktura backup export --account ${slug}\nnanofaktura backup import --owner vas@email.cz soubor.zip`}
                </pre>
              </div>
            </div>
          </FormSection>
        </>
      )}
    </SettingsPage>
  )
}
