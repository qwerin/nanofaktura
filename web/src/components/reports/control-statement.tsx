import type { ReactNode } from 'react'
import type { ControlDocumentRow, ControlStatement } from '@/api/types'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate } from '@/lib/date'
import { formatMoney } from '@/lib/money'

type RateSums = ControlStatement['a5']

/** Kontrolní hlášení (DPHKH1): oddíly A.1, A.2, A.4, A.5, B.1, B.2, B.3 — tabulky na desktopu, karty na mobilu. */
export function ControlStatementView({ data, currency }: { data: ControlStatement; currency: string }) {
  const m = (v: number) => formatMoney(v, currency)
  return (
    <section className="flex flex-col gap-4 rounded-xl border bg-card p-4 md:p-5" aria-labelledby="kh-title">
      <div>
        <h2 id="kh-title" className="text-base font-semibold tracking-tight">
          Kontrolní hlášení
        </h2>
        <p className="text-xs text-muted-foreground">Formulář DPHKH1 · na haléře</p>
      </div>

      {data.a1.length > 0 && (
        <Part code="A.1" title="Přenesení daňové povinnosti — dodavatel" hint="Kód předmětu plnění doplňte v EPO.">
          <ul className="flex flex-col divide-y rounded-lg border">
            {data.a1.map((r, i) => (
              <li key={`${r.number}-${i}`} className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-0.5 px-3 py-2 text-sm">
                <span className="font-medium tabular-nums">{r.number}</span>
                <span className="text-muted-foreground">
                  {r.customer_vat_no} · {formatDate(r.taxable_fulfillment_due)}
                </span>
                <span className="ml-auto tabular-nums">{m(r.base)}</span>
              </li>
            ))}
          </ul>
        </Part>
      )}

      {data.a2.length > 0 && (
        <Part
          code="A.2"
          title="Přijatá plnění, u nichž daň přiznáváte vy (ze zahraničí)"
          hint="Pořízení zboží a přijetí služeb z jiného státu EU, služby ze zemí mimo EU."
        >
          <DocumentRows
            rows={data.a2.map((r) => ({ ...r, vat_no: `${r.country}${r.vat_id}` }))}
            m={m}
            partyLabel="DIČ dodavatele"
            empty=""
          />
        </Part>
      )}

      <Part code="A.4" title="Prodeje plátcům nad 10 000 Kč" hint="Každý doklad zvlášť, s DIČ odběratele.">
        <DocumentRows rows={data.a4} m={m} partyLabel="DIČ odběratele" empty="Žádné doklady nad 10 000 Kč." />
      </Part>

      <Part code="A.5" title="Ostatní prodeje" hint="Doklady do 10 000 Kč a prodeje neplátcům — souhrnně.">
        <Sums sums={data.a5} m={m} />
      </Part>

      {data.b1.length > 0 && (
        <Part
          code="B.1"
          title="Přenesení daňové povinnosti — odběratel"
          hint="Tuzemské přenesení (§ 92a). Kód předmětu plnění doplňte v EPO."
        >
          <DocumentRows rows={data.b1.map((r) => ({ ...r, vat_no: r.supplier_vat_no }))} m={m} partyLabel="DIČ dodavatele" empty="" />
        </Part>
      )}

      <Part code="B.2" title="Nákupy od plátců nad 10 000 Kč" hint="Evidenční číslo dokladu dodavatele a jeho DIČ.">
        <DocumentRows rows={data.b2} m={m} partyLabel="DIČ dodavatele" empty="Žádné přijaté doklady nad 10 000 Kč." />
      </Part>

      <Part code="B.3" title="Ostatní nákupy" hint="Přijaté doklady do 10 000 Kč — souhrnně.">
        <Sums sums={data.b3} m={m} />
      </Part>
    </section>
  )
}

function Part({ code, title, hint, children }: { code: string; title: string; hint: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-baseline gap-2">
        <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs font-semibold tabular-nums">{code}</span>
        <div className="min-w-0">
          <h3 className="text-sm font-medium">{title}</h3>
          <p className="text-xs text-muted-foreground">{hint}</p>
        </div>
      </div>
      {children}
    </div>
  )
}

function DocumentRows({
  rows,
  m,
  partyLabel,
  empty,
}: {
  rows: Pick<ControlDocumentRow, 'number' | 'vat_no' | 'taxable_fulfillment_due' | 'basic' | 'reduced'>[]
  m: (v: number) => string
  partyLabel: string
  empty: string
}) {
  if (rows.length === 0) return <p className="rounded-lg border border-dashed px-3 py-3 text-sm text-muted-foreground">{empty}</p>
  const hasReduced = rows.some((r) => r.reduced.base !== 0 || r.reduced.vat !== 0)
  return (
    <>
      {/* Mobil: karty */}
      <ul className="flex flex-col gap-2 md:hidden">
        {rows.map((r, i) => (
          <li key={`${r.number}-${i}`} className="flex flex-col gap-1.5 rounded-lg border p-3 text-sm">
            <div className="flex items-baseline justify-between gap-2">
              <span className="truncate font-medium tabular-nums">{r.number}</span>
              <span className="shrink-0 text-xs text-muted-foreground tabular-nums">DUZP {formatDate(r.taxable_fulfillment_due)}</span>
            </div>
            <div className="text-xs text-muted-foreground">
              {partyLabel}: <span className="text-foreground tabular-nums">{r.vat_no}</span>
            </div>
            <RateLine label="21 %" pair={r.basic} m={m} />
            {(r.reduced.base !== 0 || r.reduced.vat !== 0) && <RateLine label="12 %" pair={r.reduced} m={m} />}
          </li>
        ))}
      </ul>
      {/* Desktop: tabulka */}
      <div className="hidden overflow-hidden rounded-lg border md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Doklad</TableHead>
              <TableHead>{partyLabel}</TableHead>
              <TableHead>DUZP</TableHead>
              <TableHead className="text-right">Základ 21 %</TableHead>
              <TableHead className="text-right">Daň 21 %</TableHead>
              {hasReduced && <TableHead className="text-right">Základ 12 %</TableHead>}
              {hasReduced && <TableHead className="text-right">Daň 12 %</TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={`${r.number}-${i}`} className="tabular-nums">
                <TableCell className="font-medium">{r.number}</TableCell>
                <TableCell>{r.vat_no}</TableCell>
                <TableCell className="text-muted-foreground">{formatDate(r.taxable_fulfillment_due)}</TableCell>
                <TableCell className="text-right">{m(r.basic.base)}</TableCell>
                <TableCell className="text-right">{m(r.basic.vat)}</TableCell>
                {hasReduced && <TableCell className="text-right">{m(r.reduced.base)}</TableCell>}
                {hasReduced && <TableCell className="text-right">{m(r.reduced.vat)}</TableCell>}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  )
}

function RateLine({ label, pair, m }: { label: string; pair: { base: number; vat: number }; m: (v: number) => string }) {
  return (
    <div className="flex items-baseline justify-between gap-2 tabular-nums">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span>
        {m(pair.base)} <span className="text-muted-foreground">+ DPH</span> {m(pair.vat)}
      </span>
    </div>
  )
}

function Sums({ sums, m }: { sums: RateSums; m: (v: number) => string }) {
  const rows = [
    { label: 'Základní sazba 21 %', pair: sums.basic },
    { label: 'Snížená sazba 12 %', pair: sums.reduced },
  ]
  return (
    <dl className="grid gap-2 sm:grid-cols-2">
      {rows.map((r) => (
        <div key={r.label} className="flex flex-col gap-0.5 rounded-lg border px-3 py-2">
          <dt className="text-xs text-muted-foreground">{r.label}</dt>
          <dd className="flex items-baseline justify-between gap-2 text-sm tabular-nums">
            <span>
              <span className="text-xs text-muted-foreground">Základ </span>
              {m(r.pair.base)}
            </span>
            <span>
              <span className="text-xs text-muted-foreground">Daň </span>
              {m(r.pair.vat)}
            </span>
          </dd>
        </div>
      ))}
    </dl>
  )
}
