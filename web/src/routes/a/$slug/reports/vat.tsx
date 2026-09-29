import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { FileCodeIcon, ReceiptTextIcon, SettingsIcon, TriangleAlertIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { z } from 'zod'
import { errorMessage, isApiError } from '@/api/errors'
import { accountQueries } from '@/api/queries/accounts'
import { reportQueries, vatXmlUrl } from '@/api/queries/reports'
import type { Account, VatReport } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { downloadFile } from '@/components/export/download'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ControlStatementView } from '@/components/reports/control-statement'
import { EpoGuide } from '@/components/reports/epo-guide'
import { ReportsNav, Stepper } from '@/components/reports/reports-nav'
import { VatReturnTable } from '@/components/reports/vat-return-table'
import { translateVatWarning, vatBalance } from '@/components/reports/vat-rows'
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDate, todayISO } from '@/lib/date'
import { formatMoney } from '@/lib/money'
import { cn } from '@/lib/utils'
import {
  defaultVatPeriod,
  formatVatPeriod,
  isVatPeriodClosed,
  parseVatPeriod,
  shiftVatPeriod,
  vatFilingDeadline,
  vatPeriodLabel,
  vatPeriodOf,
  type VatPeriod,
} from '@/lib/vat-period'

const searchSchema = z.object({
  period: z
    .string()
    .refine((s) => parseVatPeriod(s) !== null)
    .optional()
    .catch(undefined),
})

export const Route = createFileRoute('/a/$slug/reports/vat')({
  validateSearch: searchSchema,
  loader: ({ context, params }) => context.queryClient.ensureQueryData(accountQueries.detail(params.slug)),
  head: () => ({ meta: [{ title: 'DPH · NanoFaktura' }] }),
  component: VatPage,
})

function VatPage() {
  const { slug } = Route.useParams()
  const account = useQuery(accountQueries.detail(slug))

  const header = (children?: ReactNode) => (
    <PageHeader title="Přehledy" description="Podklady pro přiznání k DPH a kontrolní hlášení.">
      <div className="flex flex-col gap-3 pt-1 md:flex-row md:items-center md:justify-between md:pt-0">
        <ReportsNav slug={slug} />
        {children}
      </div>
    </PageHeader>
  )

  if (account.isError) {
    return (
      <>
        {header()}
        <PageError error={account.error} reset={() => void account.refetch()} />
      </>
    )
  }
  if (!account.data) {
    return (
      <>
        {header()}
        <PageBody>
          <Skeleton className="h-64 w-full rounded-xl" />
        </PageBody>
      </>
    )
  }
  if (account.data.vat_mode !== 'vat_payer') {
    return (
      <>
        {header()}
        <PageBody>
          <NotVatPayer slug={slug} account={account.data} />
        </PageBody>
      </>
    )
  }
  return <VatReportView slug={slug} account={account.data} header={header} />
}

function NotVatPayer({ slug, account }: { slug: string; account: Account }) {
  const { canManageSettings } = useCurrentAccount()
  return (
    <EmptyState
      icon={ReceiptTextIcon}
      title="DPH se vás netýká"
      description={
        account.vat_mode === 'identified_person'
          ? 'Identifikovaná osoba nepodává běžné přiznání z vlastních tuzemských plnění — jen z přijatých služeb a zboží z EU, které aplikace zatím neeviduje. Podklady pro přiznání a kontrolní hlášení jsou jen pro plátce DPH.'
          : 'Účet je nastavený jako neplátce DPH. Podklady pro přiznání k DPH a kontrolní hlášení jsou jen pro plátce. Pokud jste se plátcem stali, změňte režim DPH v nastavení firmy.'
      }
      action={
        canManageSettings && (
          <ButtonLink to="/a/$slug/settings/company" params={{ slug }} variant="outline">
            <SettingsIcon data-icon="inline-start" />
            Nastavení firmy
          </ButtonLink>
        )
      }
      className="bg-card"
    />
  )
}

function VatReportView({
  slug,
  account,
  header,
}: {
  slug: string
  account: Account
  header: (children?: ReactNode) => ReactNode
}) {
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const today = todayISO()
  const period: VatPeriod = parseVatPeriod(search.period) ?? defaultVatPeriod(account.vat_period, today)
  const periodStr = formatVatPeriod(period)
  const report = useQuery(reportQueries.vat(slug, periodStr))
  const current = vatPeriodOf(period.kind, today)

  const setPeriod = (p: VatPeriod) => void navigate({ search: { period: formatVatPeriod(p) }, replace: true })
  const switchKind = (kind: 'month' | 'quarter') => {
    if (kind === period.kind) return
    setPeriod(
      period.kind === 'month'
        ? { kind: 'quarter', year: period.year, quarter: Math.ceil(period.month / 3) }
        : { kind: 'month', year: period.year, month: period.quarter * 3 },
    )
  }

  const picker = (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
      <Tabs value={period.kind} onValueChange={(v) => switchKind(v as 'month' | 'quarter')}>
        <TabsList className="h-11! w-full sm:w-auto md:h-8!" aria-label="Druh období">
          <TabsTrigger value="month" className="px-3">
            Měsíc
          </TabsTrigger>
          <TabsTrigger value="quarter" className="px-3">
            Čtvrtletí
          </TabsTrigger>
        </TabsList>
      </Tabs>
      <Stepper
        label="Zdaňovací období"
        prevLabel="Předchozí období"
        nextLabel="Další období"
        onPrev={() => setPeriod(shiftVatPeriod(period, -1))}
        onNext={() => setPeriod(shiftVatPeriod(period, 1))}
        nextDisabled={periodStr >= formatVatPeriod(current)}
      >
        <span className="inline-block min-w-36 first-letter:uppercase">{vatPeriodLabel(period)}</span>
      </Stepper>
    </div>
  )

  return (
    <>
      {header(picker)}
      <PageBody className="flex flex-col gap-4 md:gap-6">
        {period.kind !== account.vat_period && (
          <p className="text-sm text-muted-foreground">
            Účet má v nastavení {account.vat_period === 'month' ? 'měsíční' : 'čtvrtletní'} zdaňovací období — zobrazujete{' '}
            {period.kind === 'month' ? 'měsíc' : 'čtvrtletí'}.
          </p>
        )}
        {report.isError ? (
          <PageError error={report.error} reset={() => void report.refetch()} />
        ) : !report.data ? (
          <div className="flex flex-col gap-4">
            <Skeleton className="h-32 w-full rounded-xl" />
            <Skeleton className="h-96 w-full rounded-xl" />
          </div>
        ) : (
          <ReportBody slug={slug} account={account} report={report.data} period={period} today={today} />
        )}
      </PageBody>
    </>
  )
}

function ReportBody({
  slug,
  account,
  report,
  period,
  today,
}: {
  slug: string
  account: Account
  report: VatReport
  period: VatPeriod
  today: string
}) {
  const currency = report.currency
  const balance = vatBalance(report.return)
  const deadline = vatFilingDeadline(period)
  const closed = isVatPeriodClosed(period, today)
  const missingSetup = !account.c_ufo || !/^CZ\d{8,10}$/i.test(account.vat_no)
  const empty =
    report.return.r62 === 0 &&
    report.return.r63 === 0 &&
    report.return.r21 === 0 &&
    report.return.r25 === 0 &&
    report.return.r26 === 0

  return (
    <>
      {/* Souhrn + stažení */}
      <section className="grid gap-4 rounded-xl border bg-card p-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-center md:p-5">
        <div className="flex flex-col gap-1">
          <span className="text-sm text-muted-foreground">
            {formatDate(report.from)} – {formatDate(report.to)}
          </span>
          <span className="text-sm font-medium">
            {balance > 0 ? 'Vlastní daň k zaplacení' : balance < 0 ? 'Nadměrný odpočet (vrátí FÚ)' : 'Daňová povinnost'}
          </span>
          <span
            className={cn(
              'text-3xl font-semibold tracking-tight tabular-nums',
              balance < 0 && 'text-success',
            )}
          >
            {formatMoney(Math.abs(balance), currency)}
          </span>
          <span className="text-sm text-muted-foreground">
            {closed ? (
              <>
                Podat a zaplatit do <strong className="font-medium text-foreground">{formatDate(deadline)}</strong>
              </>
            ) : (
              'Období ještě neskončilo — čísla se mohou měnit.'
            )}
          </span>
        </div>
        <XmlDownloads slug={slug} period={formatVatPeriod(period)} disabled={missingSetup} />
      </section>

      {missingSetup && (
        <Alert>
          <SettingsIcon />
          <AlertTitle>Pro export do EPO doplňte údaje firmy</AlertTitle>
          <AlertDescription>
            XML pro finanční úřad potřebuje {!account.c_ufo && 'kód finančního úřadu'}
            {!account.c_ufo && !/^CZ\d{8,10}$/i.test(account.vat_no) && ' a '}
            {!/^CZ\d{8,10}$/i.test(account.vat_no) && 'české DIČ (CZ…)'}. Najdete je v sekci „Daně a DPH“.
          </AlertDescription>
          <AlertAction>
            <ButtonLink to="/a/$slug/settings/company" params={{ slug }} size="sm" variant="outline">
              Doplnit
            </ButtonLink>
          </AlertAction>
        </Alert>
      )}

      {report.warnings.length > 0 && (
        <Alert className="border-warning/50 bg-warning/10">
          <TriangleAlertIcon className="text-warning-foreground dark:text-warning" />
          <AlertTitle>
            {report.warnings.length === 1 ? 'Jeden doklad vyžaduje pozornost' : `${report.warnings.length} upozornění k dokladům`}
          </AlertTitle>
          <AlertDescription>
            <ul className="mt-1 flex list-disc flex-col gap-1 pl-4">
              {report.warnings.map((w, i) => (
                <li key={i}>{translateVatWarning(w)}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {empty && (
        <p className="rounded-xl border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
          V tomto období nejsou žádné doklady s DPH. I nulové přiznání je ale potřeba podat.
        </p>
      )}

      <div className="flex flex-col gap-4 md:gap-6">
        <VatReturnTable data={report.return} currency={currency} />
        <ControlStatementView data={report.control} currency={currency} />
      </div>

      <EpoGuide deadline={deadline} />
    </>
  )
}

function XmlDownloads({ slug, period, disabled }: { slug: string; period: string; disabled: boolean }) {
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const { canManageSettings } = useCurrentAccount()

  const download = async (form: 'dphdp3' | 'dphkh1') => {
    setBusy(form)
    setError(null)
    try {
      await downloadFile(vatXmlUrl(slug, form, period), `${form}-${period}.xml`)
    } catch (err) {
      setError(
        isApiError(err) && err.status === 409
          ? 'Chybí kód finančního úřadu nebo české DIČ v nastavení firmy.'
          : errorMessage(err),
      )
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="flex flex-col gap-2 md:items-end">
      <div className="grid gap-2 sm:grid-cols-2 md:flex">
        <Button onClick={() => void download('dphdp3')} disabled={disabled || busy !== null}>
          {busy === 'dphdp3' ? <Spinner data-icon="inline-start" /> : <FileCodeIcon data-icon="inline-start" />}
          Přiznání (DPHDP3)
        </Button>
        <Button variant="outline" onClick={() => void download('dphkh1')} disabled={disabled || busy !== null}>
          {busy === 'dphkh1' ? <Spinner data-icon="inline-start" /> : <FileCodeIcon data-icon="inline-start" />}
          Kontrolní hlášení (DPHKH1)
        </Button>
      </div>
      <span className="text-xs text-muted-foreground">XML pro načtení do EPO, řádné podání</span>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}{' '}
          {canManageSettings && (
            <ButtonLink to="/a/$slug/settings/company" params={{ slug }} variant="link" className="h-auto p-0">
              Otevřít nastavení
            </ButtonLink>
          )}
        </p>
      )}
    </div>
  )
}
