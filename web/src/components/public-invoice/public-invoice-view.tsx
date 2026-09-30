// Veřejná stránka faktury pro ZÁKAZNÍKA (ne pro uživatele aplikace): bez app shellu, v jazyce dokladu,
// mobile-first, s QR Platbou, kopírováním platebních údajů a stažením PDF/ISDOC. Má i tiskový styl.

import {
  BanIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleAlertIcon,
  CircleCheckIcon,
  CopyIcon,
  FileCodeIcon,
  FileDownIcon,
  GlobeIcon,
  MailIcon,
  PhoneIcon,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { publicInvoiceFileUrl } from '@/api/queries/public'
import type { PublicInvoice, PublicParty } from '@/api/types'
import { LogoMark } from '@/components/logo'
import { Button } from '@/components/ui/button'
import { formatQuantity } from '@/lib/quantity'
import { cn } from '@/lib/utils'
import {
  amountForCopy,
  documentTitle,
  formatAmount,
  formatIban,
  formatPublicDate,
  paymentState,
  publicLabels,
  publicLang,
  type PublicLabels,
} from './i18n'
import { QrCode } from './qr-code'

export function PublicInvoiceView({ token, invoice: inv, today }: { token: string; invoice: PublicInvoice; today: string }) {
  const lang = publicLang(inv.language)
  const t = publicLabels(lang)
  const { state, days } = paymentState(inv, today)
  const money = (v: number) => formatAmount(v, inv.currency, t)
  const date = (d: string) => formatPublicDate(d, t)
  const title = documentTitle(inv, t)
  const payable = state === 'open' || state === 'overdue'
  const bank = inv.payment_method === 'bank'
  const methodLabel = inv.payment_method === 'custom' ? inv.custom_payment_method : t.methods[inv.payment_method]
  const heroAmount = payable ? inv.remaining_amount : state === 'refund' ? -inv.remaining_amount : inv.total

  return (
    <div lang={lang} className="min-h-dvh bg-muted/40 pb-safe print:bg-white">
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-10 md:gap-5 md:pt-10 print:max-w-none print:gap-3 print:p-0">
        {/* Hlavička: dodavatel + číslo */}
        <header className="flex items-center gap-3">
          <SupplierMark name={inv.supplier.name} logoUrl={inv.logo_url} />
          <div className="min-w-0 flex-1">
            <p className="truncate text-base font-semibold tracking-tight">{inv.supplier.name}</p>
            <p className="truncate text-sm text-muted-foreground">
              {title} {t.number} <span className="font-medium text-foreground tabular-nums">{inv.number}</span>
            </p>
          </div>
        </header>

        {/* Stav */}
        <StatusBanner state={state} days={days} t={t} paidOn={inv.paid_on} date={date} />

        {/* Částka */}
        <section
          aria-label={payable ? t.toPay : t.total}
          className={cn(
            'flex flex-col gap-1 rounded-2xl border bg-card p-5 shadow-xs md:p-6 print:shadow-none',
            state === 'cancelled' && 'opacity-70',
          )}
        >
          <span className="text-sm font-medium text-muted-foreground">
            {payable ? t.toPay : state === 'refund' ? t.refund : t.total}
          </span>
          <span
            className={cn(
              'text-4xl font-semibold tracking-tight tabular-nums md:text-5xl',
              state === 'cancelled' && 'line-through decoration-2',
            )}
          >
            {money(heroAmount)}
          </span>
          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
            <span>
              <span className="text-muted-foreground">{t.due}: </span>
              <span className={cn('font-medium tabular-nums', state === 'overdue' && 'text-destructive')}>{date(inv.due_on)}</span>
            </span>
            {payable && inv.paid_amount > 0 && (
              <span className="text-muted-foreground">
                {t.alreadyPaid}: <span className="tabular-nums">{money(inv.paid_amount)}</span>
              </span>
            )}
            {inv.variable_symbol && (
              <span>
                <span className="text-muted-foreground">{t.vs}: </span>
                <span className="font-medium tabular-nums">{inv.variable_symbol}</span>
              </span>
            )}
          </div>
        </section>

        {/* Platba */}
        {payable && (
          <section aria-labelledby="pay-title" className="flex flex-col gap-4 rounded-2xl border bg-card p-5 md:p-6 print:break-inside-avoid">
            <h2 id="pay-title" className="text-base font-semibold tracking-tight">
              {t.payment}
            </h2>
            {bank && (inv.bank_account || inv.iban) ? (
              <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
                {inv.spayd && (
                  <figure className="flex flex-col items-center gap-2 self-center sm:self-start">
                    <QrCode value={inv.spayd} label={`QR Platba: ${money(inv.remaining_amount)}`} className="w-56 border sm:w-44" />
                    <figcaption className="max-w-56 text-center text-xs text-muted-foreground sm:max-w-44">
                      <span className="block font-semibold text-foreground">QR Platba</span>
                      {t.qrHint}
                    </figcaption>
                  </figure>
                )}
                <dl className="flex min-w-0 flex-1 flex-col divide-y">
                  {inv.bank_account && <CopyRow label={t.account} value={inv.bank_account} t={t} />}
                  {inv.iban && <CopyRow label={t.iban} value={formatIban(inv.iban)} copyValue={inv.iban.replace(/\s/g, '')} t={t} />}
                  {inv.swift_bic && <CopyRow label={t.swift} value={inv.swift_bic} t={t} />}
                  {inv.variable_symbol && <CopyRow label={t.vs} value={inv.variable_symbol} t={t} />}
                  <CopyRow label={t.amount} value={money(inv.remaining_amount)} copyValue={amountForCopy(inv.remaining_amount, t)} t={t} />
                </dl>
              </div>
            ) : (
              <p className="text-sm">
                <span className="text-muted-foreground">{t.method}: </span>
                <span className="font-medium">{methodLabel || t.methods.bank}</span>
              </p>
            )}
          </section>
        )}

        {/* Stažení */}
        <div className="grid grid-cols-2 gap-2 print:hidden">
          <Button
            size="lg"
            nativeButton={false}
            render={<a href={publicInvoiceFileUrl(token, 'pdf')} target="_blank" rel="noopener" />}
          >
            <FileDownIcon data-icon="inline-start" />
            {t.downloadPdf}
          </Button>
          <Button
            size="lg"
            variant="outline"
            nativeButton={false}
            title={t.isdocHint}
            render={<a href={publicInvoiceFileUrl(token, 'isdoc')} download={`${inv.number.replace(/[^A-Za-z0-9._-]/g, '-')}.isdoc`} />}
          >
            <FileCodeIcon data-icon="inline-start" />
            {t.downloadIsdoc}
          </Button>
        </div>

        {/* Strany */}
        <section className="grid gap-3 sm:grid-cols-2">
          <PartyCard title={t.supplier} party={inv.supplier} t={t} supplier />
          <PartyCard title={t.customer} party={inv.customer} t={t} />
        </section>

        {/* Údaje dokladu */}
        <section className="rounded-2xl border bg-card px-5 py-4 md:px-6">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
            <Fact label={t.issued} value={date(inv.issued_on)} />
            {inv.supplier.vat_mode === 'vat_payer' && inv.taxable_fulfillment_due && (
              <Fact label={t.taxPoint} value={date(inv.taxable_fulfillment_due)} />
            )}
            <Fact label={t.due} value={date(inv.due_on)} />
            {methodLabel && <Fact label={t.method} value={methodLabel} />}
            {inv.order_number && <Fact label={t.order} value={inv.order_number} />}
            {inv.related_number && <Fact label={t.related('').trim()} value={inv.related_number} />}
            {inv.currency !== 'CZK' && inv.exchange_rate && inv.exchange_rate !== '1' && (
              <Fact label={t.exchangeRate} value={`${inv.exchange_rate} CZK/${inv.currency}`} />
            )}
          </dl>
        </section>

        {/* Položky + součty */}
        <LinesSection inv={inv} t={t} money={money} />

        {(inv.note || inv.footer_note) && (
          <section className="flex flex-col gap-2 rounded-2xl border bg-card px-5 py-4 text-sm whitespace-pre-line md:px-6">
            {inv.note && <p>{inv.note}</p>}
            {inv.footer_note && <p className="text-muted-foreground">{inv.footer_note}</p>}
          </section>
        )}

        <footer className="flex items-center justify-center gap-2 pt-4 text-xs text-muted-foreground print:hidden">
          {t.poweredBy}
          <span className="inline-flex items-center gap-1.5 font-medium text-foreground/80">
            <LogoMark className="size-4 rounded-[4px]" />
            NanoFaktura
          </span>
        </footer>
      </div>
    </div>
  )
}

function SupplierMark({ name, logoUrl }: { name: string; logoUrl?: string }) {
  const [broken, setBroken] = useState(false)
  if (logoUrl && !broken) {
    const base = import.meta.env.VITE_API_BASE_URL ?? ''
    return (
      <img
        src={`${base}${logoUrl}`}
        alt=""
        onError={() => setBroken(true)}
        className="h-11 max-w-32 shrink-0 object-contain object-left"
      />
    )
  }
  const letters = name
    .replace(/[,.].*$/, '')
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
  return (
    <span
      aria-hidden="true"
      className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary text-base font-semibold text-primary-foreground print:border print:bg-white print:text-black"
    >
      {letters || '·'}
    </span>
  )
}

function StatusBanner({
  state,
  days,
  t,
  paidOn,
  date,
}: {
  state: ReturnType<typeof paymentState>['state']
  days: number
  t: PublicLabels
  paidOn: string
  date: (d: string) => string
}) {
  if (state === 'open') {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <span className="size-2 rounded-full bg-info" aria-hidden="true" />
        {t.dueIn(days)}
      </p>
    )
  }
  const variants = {
    paid: {
      icon: CircleCheckIcon,
      cls: 'border-success/40 bg-success/10 text-foreground [&>svg]:text-success',
      title: t.paid,
      text: paidOn ? `${t.paidOn} ${date(paidOn)}. ${t.paidThanks}` : t.paidThanks,
    },
    overdue: {
      icon: CircleAlertIcon,
      cls: 'border-destructive/40 bg-destructive/10 [&>svg]:text-destructive',
      title: t.overdue,
      text: t.overdueDays(Math.max(1, -days)),
    },
    cancelled: { icon: BanIcon, cls: 'border-border bg-muted [&>svg]:text-muted-foreground', title: t.cancelled, text: t.cancelledHint },
    uncollectible: { icon: BanIcon, cls: 'border-border bg-muted [&>svg]:text-muted-foreground', title: t.uncollectible, text: '' },
    refund: { icon: CircleAlertIcon, cls: 'border-info/40 bg-info/10 [&>svg]:text-info', title: t.refund, text: '' },
  } as const
  const v = variants[state]
  return (
    <div role="status" className={cn('flex items-start gap-3 rounded-2xl border px-4 py-3', v.cls)}>
      <v.icon className="mt-0.5 size-5 shrink-0" />
      <div className="flex flex-col">
        <span className="font-semibold">{v.title}</span>
        {v.text && <span className="text-sm text-muted-foreground">{v.text}</span>}
      </div>
    </div>
  )
}

function CopyRow({ label, value, copyValue, t }: { label: string; value: string; copyValue?: string; t: PublicLabels }) {
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (!copied) return
    const id = setTimeout(() => setCopied(false), 1500)
    return () => clearTimeout(id)
  }, [copied])
  return (
    <div className="flex items-center gap-2 py-1.5 first:pt-0 last:pb-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <dt className="text-xs text-muted-foreground">{label}</dt>
        <dd className="font-medium break-all tabular-nums select-all">{value}</dd>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="shrink-0 text-muted-foreground print:hidden"
        aria-label={copied ? t.copied : `${t.copy}: ${label}`}
        title={copied ? t.copied : t.copy}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(copyValue ?? value)
            setCopied(true)
          } catch {
            /* schránka nedostupná — hodnotu lze označit (select-all) */
          }
        }}
      >
        {copied ? <CheckIcon className="text-success" /> : <CopyIcon />}
      </Button>
      <span className="sr-only" aria-live="polite">
        {copied ? t.copied : ''}
      </span>
    </div>
  )
}

function PartyCard({ title, party: p, t, supplier }: { title: string; party: PublicParty; t: PublicLabels; supplier?: boolean }) {
  const address = [p.street, [p.zip, p.city].filter(Boolean).join(' '), p.country && p.country !== 'CZ' ? p.country : '']
    .filter(Boolean)
  return (
    <div className="flex flex-col gap-2 rounded-2xl border bg-card px-5 py-4 text-sm">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</h2>
      <div className="flex flex-col">
        <span className="font-semibold">{p.name}</span>
        {p.full_name && <span>{p.full_name}</span>}
        {address.map((line) => (
          <span key={line} className="text-muted-foreground">
            {line}
          </span>
        ))}
      </div>
      {(p.registration_no || p.vat_no || supplier) && (
        <div className="flex flex-col text-muted-foreground tabular-nums">
          {p.registration_no && (
            <span>
              {t.regNo}: <span className="text-foreground">{p.registration_no}</span>
            </span>
          )}
          {p.vat_no && (
            <span>
              {t.vatNo}: <span className="text-foreground">{p.vat_no}</span>
            </span>
          )}
          {supplier && p.vat_mode === 'non_vat_payer' && <span>{t.nonPayer}</span>}
          {supplier && p.vat_mode === 'identified_person' && <span>{t.identified}</span>}
        </div>
      )}
      {supplier && p.registered_by && <p className="text-xs text-muted-foreground">{p.registered_by}</p>}
      {supplier && (p.email || p.phone || p.web) && (
        <div className="flex flex-col gap-1 pt-1">
          {p.email && <ContactLink href={`mailto:${p.email}`} icon={<MailIcon />} text={p.email} />}
          {p.phone && <ContactLink href={`tel:${p.phone.replace(/\s/g, '')}`} icon={<PhoneIcon />} text={p.phone} />}
          {p.web && (
            <ContactLink
              href={/^https?:\/\//.test(p.web) ? p.web : `https://${p.web}`}
              icon={<GlobeIcon />}
              text={p.web.replace(/^https?:\/\//, '')}
              external
            />
          )}
        </div>
      )}
    </div>
  )
}

function ContactLink({ href, icon, text, external }: { href: string; icon: ReactNode; text: string; external?: boolean }) {
  return (
    <a
      href={href}
      {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
      className="flex min-h-8 items-center gap-2 break-all text-primary underline-offset-4 hover:underline [&>svg]:size-4 [&>svg]:shrink-0"
    >
      {icon}
      {text}
    </a>
  )
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate font-medium tabular-nums">{value}</dd>
    </div>
  )
}

function LinesSection({ inv, t, money }: { inv: PublicInvoice; t: PublicLabels; money: (v: number) => string }) {
  const payer = inv.supplier.vat_mode === 'vat_payer'
  const pct = (bps: number) => `${new Intl.NumberFormat(t.locale, { maximumFractionDigits: 2 }).format(bps / 100)} %`
  return (
    <details className="group rounded-2xl border bg-card print:break-inside-avoid [&_summary::-webkit-details-marker]:hidden" open={inv.lines.length <= 3}>
      <summary className="flex min-h-14 cursor-pointer list-none items-center justify-between gap-3 rounded-2xl px-5 py-3 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:px-6 print:hidden">
        <span className="flex flex-col">
          <span className="font-semibold">{t.items}</span>
          <span className="text-xs text-muted-foreground">{t.itemsCount(inv.lines.length)}</span>
        </span>
        <span className="flex items-center gap-2">
          <span className="font-semibold tabular-nums">{money(inv.total)}</span>
          <ChevronDownIcon className="size-5 text-muted-foreground transition-transform group-open:rotate-180" />
        </span>
      </summary>
      <div className="flex flex-col gap-4 px-5 pb-5 md:px-6">
        <ul className="flex flex-col divide-y border-t">
          {inv.lines.map((l, i) => (
            <li key={i} className="flex flex-col gap-0.5 py-3 text-sm">
              <div className="flex items-baseline justify-between gap-3">
                <span className="min-w-0 font-medium break-words">{l.name}</span>
                <span className="shrink-0 font-medium tabular-nums">{money(payer && !inv.prices_include_vat ? l.base : l.total)}</span>
              </div>
              <span className="text-xs text-muted-foreground tabular-nums">
                {formatQuantity(l.quantity)}
                {l.unit_name ? ` ${l.unit_name}` : ''} × {money(l.unit_price)}
                {payer && ` · ${t.vat} ${pct(l.vat_rate_bps)}`}
              </span>
            </li>
          ))}
        </ul>

        {payer && inv.vat_recap.length > 0 && (
          <div className="flex flex-col gap-1.5">
            <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{t.vatRecap}</h3>
            <table className="w-full text-sm tabular-nums">
              <thead>
                <tr className="text-xs text-muted-foreground">
                  <th scope="col" className="py-1 text-left font-normal">{t.rate}</th>
                  <th scope="col" className="py-1 text-right font-normal">{t.base}</th>
                  <th scope="col" className="py-1 text-right font-normal">{t.vat}</th>
                  <th scope="col" className="py-1 text-right font-normal">{t.total}</th>
                </tr>
              </thead>
              <tbody>
                {inv.vat_recap.map((r) => (
                  <tr key={r.vat_rate_bps}>
                    <td className="py-1">{pct(r.vat_rate_bps)}</td>
                    <td className="py-1 text-right">{money(r.base)}</td>
                    <td className="py-1 text-right">{money(r.vat)}</td>
                    <td className="py-1 text-right">{money(r.total)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <dl className="flex flex-col gap-1 border-t pt-3 text-sm tabular-nums">
          {payer && <Sum label={t.subtotal} value={money(inv.subtotal)} />}
          {payer && <Sum label={t.vat} value={money(inv.vat_total)} />}
          {inv.rounding !== 0 && <Sum label={t.rounding} value={money(inv.rounding)} />}
          <Sum label={t.total} value={money(inv.total)} strong />
          {inv.paid_amount !== 0 && <Sum label={t.alreadyPaid} value={money(-inv.paid_amount)} />}
          {inv.paid_amount !== 0 && inv.remaining_amount !== 0 && <Sum label={t.remaining} value={money(inv.remaining_amount)} strong />}
        </dl>
        {inv.reverse_charge && <p className="text-xs text-muted-foreground">{t.reverseCharge}</p>}
      </div>
    </details>
  )
}

function Sum({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className={cn('flex items-baseline justify-between gap-3', strong && 'text-base font-semibold')}>
      <dt className={cn(!strong && 'text-muted-foreground')}>{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}
