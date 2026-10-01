import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { CorrectionInput, CreatePaymentInput, FinalInvoiceInput, Invoice } from '@/api/types'
import { makeInvoice } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server } from '@/test/server'

const payerProforma = (overrides: Partial<Invoice> = {}) =>
  makeInvoice({
    id: 50,
    document_type: 'proforma',
    number: 'ZF2026-0001',
    status: 'sent',
    your_vat_mode: 'vat_payer',
    your_vat_no: 'CZ12345678',
    subtotal: 100_000,
    vat_total: 21_000,
    total: 121_000,
    remaining_amount: 121_000,
    ...overrides,
  })

function mockApi(inv: Invoice) {
  const calls = { payments: [] as CreatePaymentInput[], corrections: [] as CorrectionInput[], finals: [] as FinalInvoiceInput[] }
  server.use(
    http.get(`${API}/api/accounts/:slug/invoices/:id`, ({ params }) =>
      HttpResponse.json(Number(params.id) === inv.id ? inv : makeInvoice({ id: Number(params.id), number: `X-${String(params.id)}` })),
    ),
    http.post(`${API}/api/accounts/:slug/invoices/:id/payments`, async ({ request }) => {
      const body = (await request.json()) as CreatePaymentInput
      calls.payments.push(body)
      return HttpResponse.json(
        {
          invoice: { ...inv, paid_amount: body.amount ?? 0, remaining_amount: inv.remaining_amount - (body.amount ?? 0) },
          payment: { id: 1, invoice_id: inv.id, amount: body.amount ?? 0, paid_on: body.paid_on ?? '', note: '', created_at: '' },
          tax_document_id: inv.your_vat_mode === 'vat_payer' && inv.document_type === 'proforma' ? 61 : undefined,
        },
        { status: 201 },
      )
    }),
    http.post(`${API}/api/accounts/:slug/invoices/:id/correction`, async ({ request }) => {
      const body = (await request.json()) as CorrectionInput
      calls.corrections.push(body)
      if (!body.correction_reason) {
        return problem(422, 'validation failed', { errors: [{ location: 'body.correction_reason', message: 'důvod chybí' }] })
      }
      return HttpResponse.json(makeInvoice({ id: 70, document_type: 'correction', number: 'D2026-0001' }), { status: 201 })
    }),
    http.post(`${API}/api/accounts/:slug/invoices/:id/final-invoice`, async ({ request }) => {
      calls.finals.push((await request.json()) as FinalInvoiceInput)
      return HttpResponse.json(makeInvoice({ id: 80, number: '2026-0080' }), { status: 201 })
    }),
  )
  return calls
}

async function openDetail(inv: Invoice) {
  const app = await renderApp(`/a/firma/invoices/${inv.id}`)
  await screen.findByRole('heading', { level: 1, name: new RegExp(inv.number) })
  return app
}

describe('zálohy — platba', () => {
  it('částečná platba nepředvolí vyúčtování; plátce vidí daňový doklad k platbě', async () => {
    const inv = payerProforma()
    const calls = mockApi(inv)
    const { user } = await openDetail(inv)

    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(within(dialog).getByText(/automaticky vystaví daňový doklad/)).toBeInTheDocument()
    const final = within(dialog).getByRole('switch', { name: 'Vystavit konečnou fakturu' })
    expect(final).toBeChecked() // celá zbývající částka

    const amount = within(dialog).getByLabelText('Částka (CZK)')
    await user.clear(amount)
    await user.type(amount, '400')
    expect(final).not.toBeChecked()

    // vědomá volba: zbytek zůstane na konečné faktuře
    await user.click(final)
    expect(within(dialog).getByText(/zůstane k úhradě na ní/)).toBeInTheDocument()
    await user.click(final)

    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    await waitFor(() => expect(calls.payments).toHaveLength(1))
    expect(calls.payments[0]).toMatchObject({ amount: 40_000, create_final_invoice: false })
    expect(await screen.findByText('Platba přidána a vystaven daňový doklad k přijaté platbě')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zobrazit daňový doklad' })).toBeInTheDocument()
  })

  it('platba musí mít znaménko zbývající částky', async () => {
    const inv = makeInvoice()
    const calls = mockApi(inv)
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    const amount = within(dialog).getByLabelText('Částka (CZK)')
    await user.clear(amount)
    await user.type(amount, '-100')
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    expect(await within(dialog).findByText('Platba musí být kladná (zbývá doplatit)')).toBeInTheDocument()
    expect(calls.payments).toHaveLength(0)
  })
})

describe('zálohy — vyúčtování a související doklady', () => {
  it('vyúčtovaná proforma: odkaz na fakturu místo zbývající částky, bez platby', async () => {
    const inv = payerProforma({
      status: 'paid',
      paid_amount: 40_000,
      remaining_amount: 81_000,
      payments: [{ id: 1, invoice_id: 50, amount: 40_000, paid_on: '2026-03-05', note: '', created_at: '', tax_document_id: 61 }],
      related_documents: [
        { id: 61, document_type: 'tax_document', number: 'ZD2026-0001', status: 'paid', total: 40_000 },
        { id: 80, document_type: 'invoice', number: '2026-0080', status: 'sent', total: 121_000 },
      ],
    })
    mockApi(inv)
    const { user } = await openDetail(inv)
    expect(screen.getAllByText(/Vyúčtováno fakturou/).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('link', { name: '2026-0080' })[0]).toHaveAttribute('href', '/a/firma/invoices/80')
    expect(screen.getByRole('link', { name: 'ZD2026-0001' })).toHaveAttribute('href', '/a/firma/invoices/61')
    expect(screen.getByRole('link', { name: 'Daňový doklad k platbě' })).toHaveAttribute('href', '/a/firma/invoices/61')
    expect(screen.queryByRole('button', { name: 'Přidat platbu' })).not.toBeInTheDocument()
    expect(screen.queryByText(/^zbývá/)).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).queryByRole('menuitem', { name: 'Vystavit vyúčtování' })).not.toBeInTheDocument()
  })

  it('vystavení vyúčtování z menu přejde na novou fakturu', async () => {
    const inv = payerProforma()
    const calls = mockApi(inv)
    const { user, router } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Vystavit vyúčtování' }))
    const dialog = await screen.findByRole('dialog', { name: 'Vystavit vyúčtování' })
    const issued = within(dialog).getByLabelText('Datum vystavení')
    await user.clear(issued)
    await user.type(issued, '2026-03-20')
    // DUZP drží krok s datem vystavení
    expect(within(dialog).getByLabelText('Datum zdanitelného plnění')).toHaveValue('2026-03-20')
    await user.click(within(dialog).getByRole('button', { name: 'Vystavit' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/80'))
    expect(calls.finals[0]).toEqual({ issued_on: '2026-03-20', taxable_fulfillment_due: '2026-03-20' })
  })

  it('vyúčtovací faktura: odpočet záloh, převzatou platbu nejde smazat', async () => {
    const inv = makeInvoice({
      id: 80,
      number: '2026-0080',
      your_vat_mode: 'vat_payer',
      total: 121_000,
      paid_amount: 40_000,
      remaining_amount: 81_000,
      related_id: 50,
      payments: [{ id: 9, invoice_id: 80, amount: 40_000, paid_on: '2026-03-05', note: '', created_at: '', source_payment_id: 1 }],
      deposits: [
        {
          tax_document_id: 61,
          number: 'ZD2026-0001',
          taxable_fulfillment_due: '2026-03-05',
          total: 40_000,
          vat_recap: [{ vat_rate_bps: 2100, base: 33_058, vat: 6_942, total: 40_000 }],
        },
      ],
    })
    mockApi(inv)
    await openDetail(inv)
    expect(screen.getByRole('heading', { name: 'Odpočet záloh' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'ZD2026-0001' })).toHaveAttribute('href', '/a/firma/invoices/61')
    expect(screen.getByText('Zálohová faktura')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Zobrazit doklad' })).toHaveAttribute('href', '/a/firma/invoices/50')
    expect(screen.getByRole('button', { name: 'Smazat platbu' })).toBeDisabled()
    expect(screen.getByLabelText('Platba převzatá ze zálohové faktury')).toBeInTheDocument()
  })

  it('daňový doklad k platbě nejde upravit, duplikovat ani smazat', async () => {
    const inv = makeInvoice({ id: 61, document_type: 'tax_document', number: 'ZD2026-0001', status: 'paid', remaining_amount: 0 })
    mockApi(inv)
    const { user } = await openDetail(inv)
    expect(screen.queryByRole('link', { name: 'Upravit' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    const menu = await screen.findByRole('menu')
    for (const name of ['Duplikovat', 'Smazat', 'Stornovat', 'Uložit jako šablonu', 'Vystavit opravný doklad']) {
      expect(within(menu).queryByRole('menuitem', { name })).not.toBeInTheDocument()
    }
  })
})

describe('opravný doklad', () => {
  it('plátce musí uvést důvod; návrh ho vyplní', async () => {
    const inv = makeInvoice({ your_vat_mode: 'vat_payer', status: 'sent' })
    const calls = mockApi(inv)
    const { user, router } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Vystavit opravný doklad' }))
    const dialog = await screen.findByRole('dialog', { name: 'Vystavit opravný doklad' })
    await user.click(within(dialog).getByRole('button', { name: 'Vystavit' }))
    expect(await within(dialog).findByText('Uveďte důvod opravy')).toBeInTheDocument()
    expect(calls.corrections).toHaveLength(0)

    await user.click(within(dialog).getByRole('button', { name: 'Vrácení zboží' }))
    expect(within(dialog).getByLabelText('Důvod opravy')).toHaveValue('Vrácení zboží')
    await user.click(within(dialog).getByRole('button', { name: 'Vystavit' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/70/edit'))
    expect(calls.corrections[0]).toEqual({ correction_reason: 'Vrácení zboží' })
  })

  it('neplátce může důvod vynechat', async () => {
    const inv = makeInvoice({ status: 'sent' })
    const calls = mockApi(inv)
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Vystavit opravný doklad' }))
    const dialog = await screen.findByRole('dialog', { name: 'Vystavit opravný doklad' })
    expect(within(dialog).getByLabelText('Důvod opravy (nepovinné)')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Vystavit' }))
    await waitFor(() => expect(calls.corrections).toEqual([{}]))
    // mock vyžaduje důvod → 422 se ukáže u pole
    expect(await within(dialog).findByText('důvod chybí')).toBeInTheDocument()
  })
})
