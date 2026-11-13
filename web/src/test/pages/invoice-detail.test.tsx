import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { CreatePaymentInput, EmailLog, Invoice } from '@/api/types'
import { makeEETSale, makeEETSettings, makeInvoice } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'
import { setMobile } from '@/test/viewport'

type SendBody = { kind: string; to: string[]; cc: string[]; subject?: string; attach_pdf: boolean; attach_isdoc: boolean }

function mockInvoice(inv: Invoice) {
  const calls = { payments: [] as CreatePaymentInput[], sends: [] as SendBody[], previews: [] as URLSearchParams[] }
  server.use(
    http.get(`${API}/api/accounts/:slug/invoices/:id`, () => HttpResponse.json(inv)),
    http.post(`${API}/api/accounts/:slug/invoices/:id/payments`, async ({ request }) => {
      const body = (await request.json()) as CreatePaymentInput
      calls.payments.push(body)
      if (body.amount === 99) {
        return problem(422, 'validation failed', { errors: [{ location: 'body.amount', message: 'částka převyšuje zbývající' }] })
      }
      const paid = { ...inv, status: 'paid' as const, paid_amount: inv.total, remaining_amount: 0 }
      return HttpResponse.json(
        {
          invoice: paid,
          payment: { id: 1, invoice_id: inv.id, amount: body.amount ?? 0, paid_on: body.paid_on ?? '', note: '', created_at: '' },
          final_invoice_id: body.create_final_invoice ? 77 : undefined,
        },
        { status: 201 },
      )
    }),
    http.get(`${API}/api/accounts/:slug/email-templates/preview`, ({ request }) => {
      const q = new URL(request.url).searchParams
      calls.previews.push(q)
      const lang = q.get('lang') ?? inv.language
      return HttpResponse.json({
        kind: q.get('kind') ?? 'invoice',
        lang,
        to: inv.client_email ? [inv.client_email] : [],
        cc: [],
        subject: `${lang}: Faktura ${inv.number}`,
        body: `Text (${lang})`,
      })
    }),
    http.post(`${API}/api/accounts/:slug/invoices/:id/send`, async ({ request }) => {
      const body = (await request.json()) as SendBody
      calls.sends.push(body)
      if (body.to.includes('bounce@example.cz')) return problem(502, 'SMTP 550 mailbox unavailable')
      const log: EmailLog = {
        id: 1,
        invoice_id: inv.id,
        kind: 'invoice',
        to: body.to,
        cc: body.cc,
        subject: body.subject ?? '',
        body: '',
        attachments: [],
        automatic: false,
        error: '',
        reminder_step: 0,
        created_at: '',
      }
      return HttpResponse.json(log)
    }),
  )
  return calls
}

/** Akce hlavičky: tlačítko, nebo odkaz vykreslený jako tlačítko (Base UI `render`). */
function action(name: string, container: HTMLElement = document.body) {
  const q = within(container)
  return [...q.queryAllByRole('button', { name }), ...q.queryAllByRole('link', { name }), ...q.queryAllByRole('menuitem', { name })][0] ?? null
}

async function openDetail(inv: Invoice) {
  const app = await renderApp(`/a/firma/invoices/${inv.id}`)
  await screen.findByRole('heading', { level: 1, name: new RegExp(inv.number) })
  return app
}

describe('detail faktury — platba', () => {
  it('předvyplní zbývající částku a po zaplacení zavře dialog', async () => {
    const inv = makeInvoice({ remaining_amount: 250_000, total: 1_000_000, paid_amount: 750_000 })
    const calls = mockInvoice(inv)
    const { user } = await openDetail(inv)

    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(within(dialog).getByLabelText('Částka (CZK)')).toHaveValue('2500,00')
    // u běžné faktury se konečná faktura nenabízí
    expect(within(dialog).queryByRole('switch', { name: 'Vystavit konečnou fakturu' })).not.toBeInTheDocument()

    await user.clear(within(dialog).getByLabelText('Částka (CZK)'))
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    expect(await within(dialog).findByText('Zadejte nenulovou částku')).toBeInTheDocument()
    expect(calls.payments).toHaveLength(0)

    await user.type(within(dialog).getByLabelText('Částka (CZK)'), '2 500,00')
    await user.type(within(dialog).getByLabelText('Poznámka'), ' hotově ')
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Přidat platbu' })).not.toBeInTheDocument())
    expect(calls.payments[0]).toMatchObject({ amount: 250_000, note: 'hotově' })
    expect(calls.payments[0]!.create_final_invoice).toBeUndefined()
    expect(await screen.findByText('Faktura je uhrazená')).toBeInTheDocument()
  })

  it('422 z API ukáže u pole částky', async () => {
    const inv = makeInvoice()
    mockInvoice(inv)
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    await user.clear(within(dialog).getByLabelText('Částka (CZK)'))
    await user.type(within(dialog).getByLabelText('Částka (CZK)'), '0,99')
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    expect(await within(dialog).findByText('částka převyšuje zbývající')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('Částka (CZK)')).toHaveAttribute('aria-invalid', 'true')
  })

  it('zálohová faktura: platba vystaví konečnou fakturu a přejde na ni', async () => {
    const inv = makeInvoice({ document_type: 'proforma', number: 'ZF-2026-0001' })
    const calls = mockInvoice(inv)
    const { user, router } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(within(dialog).getByRole('switch', { name: 'Vystavit konečnou fakturu' })).toBeChecked()
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/77'))
    expect(calls.payments[0]!.create_final_invoice).toBe(true)
  })
})

describe('detail faktury — odeslání e-mailem', () => {
  it('výchozí jazyk podle dokladu, příjemce z odběratele, kopie a přílohy', async () => {
    const inv = makeInvoice({ status: 'open', language: 'en' })
    const calls = mockInvoice(inv)
    const { user } = await openDetail(inv)

    // neodeslaná faktura → odeslání je hlavní akce
    await user.click(screen.getByRole('button', { name: 'Odeslat e-mailem' }))
    const dialog = await screen.findByRole('dialog', { name: `Odeslat ${inv.number} e-mailem` })
    expect(await within(dialog).findByLabelText('Předmět')).toHaveValue('en: Faktura 2026-0001')
    expect(within(dialog).getByRole('radio', { name: 'Angličtina' })).toHaveAttribute('aria-checked', 'true')
    expect(within(dialog).getByText('acme@example.cz')).toBeInTheDocument()
    expect(within(dialog).getByText('Po odeslání se faktura označí jako odeslaná.')).toBeInTheDocument()
    expect(calls.previews[0]!.get('lang')).toBeNull()

    // přepnutí jazyka načte text šablony v češtině a upozorní na rozdíl proti PDF
    await user.click(within(dialog).getByRole('radio', { name: 'Čeština' }))
    await waitFor(() => expect(within(dialog).getByLabelText('Předmět')).toHaveValue('cs: Faktura 2026-0001'))
    expect(within(dialog).getByText(/e-mail nebude ve stejném jazyce jako PDF/)).toBeInTheDocument()

    // příjemci jako čipy: čárka potvrdí, neplatná adresa zablokuje odeslání
    await user.click(within(dialog).getByRole('button', { name: '+ Kopie' }))
    await user.type(within(dialog).getByLabelText('Kopie'), 'ucetni@example.cz,nejaka-blbost,')
    await user.click(within(dialog).getByRole('button', { name: 'Odeslat' }))
    expect(await within(dialog).findByText('Opravte zvýrazněné adresy.')).toBeInTheDocument()
    expect(calls.sends).toHaveLength(0)
    await user.click(within(dialog).getByRole('button', { name: 'Odebrat nejaka-blbost' }))

    await user.click(within(dialog).getByRole('switch', { name: 'ISDOC' }))
    await user.click(within(dialog).getByRole('button', { name: 'Odeslat' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Odeslat .* e-mailem/ })).not.toBeInTheDocument())
    expect(calls.sends[0]).toMatchObject({
      kind: 'invoice',
      to: ['acme@example.cz'],
      cc: ['ucetni@example.cz'],
      subject: 'cs: Faktura 2026-0001',
      attach_pdf: true,
      attach_isdoc: true,
    })
  })

  it('bez e-mailu odběratele vyžaduje adresu; chyba SMTP se ukáže v dialogu', async () => {
    const inv = makeInvoice({ status: 'open', client_email: '' })
    const calls = mockInvoice(inv)
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Odeslat e-mailem' }))
    const dialog = await screen.findByRole('dialog', { name: /e-mailem/ })
    expect(await within(dialog).findByText('Faktura nemá e-mail odběratele — zadejte adresu ručně.')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Odeslat' }))
    expect(await within(dialog).findByText('Zadejte aspoň jednu adresu.')).toBeInTheDocument()

    await user.type(within(dialog).getByLabelText('Komu'), 'bounce@example.cz{Enter}')
    await user.click(within(dialog).getByRole('button', { name: 'Odeslat' }))
    expect(await within(dialog).findByText('E-mail se nepodařilo odeslat')).toBeInTheDocument()
    expect(within(dialog).getByText('SMTP 550 mailbox unavailable')).toBeInTheDocument()
    expect(calls.sends).toHaveLength(1)
  })

  it('uhrazená faktura nabízí poděkování bez PDF', async () => {
    const inv = makeInvoice({ status: 'paid', paid_amount: 1_000_000, remaining_amount: 0, sent_at: '2026-03-02T08:00:00Z' })
    mockInvoice(inv)
    const { user } = await openDetail(inv)
    // odeslaná faktura → odeslání je v menu „Další akce“
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Odeslat e-mailem' }))
    const dialog = await screen.findByRole('dialog', { name: /e-mailem/ })
    await user.click(within(dialog).getByRole('radio', { name: 'Poděkování' }))
    expect(within(dialog).getByRole('switch', { name: 'PDF dokladu' })).not.toBeChecked()
    expect(within(dialog).queryByRole('switch', { name: 'ISDOC' })).not.toBeInTheDocument()
  })
})

describe('detail faktury — oprávnění', () => {
  it('účetní nevidí žádné měnící akce, jen PDF', async () => {
    setSession('accountant')
    const inv = makeInvoice({ status: 'open' })
    mockInvoice(inv)
    const { user } = await openDetail(inv)
    expect(action('Otevřít PDF')).toBeInTheDocument()
    for (const name of ['Přidat platbu', 'Odeslat e-mailem', 'Upravit']) expect(action(name)).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).getByRole('menuitem', { name: 'Stáhnout PDF' })).toBeInTheDocument()
    for (const name of ['Duplikovat', 'Stornovat', 'Smazat', 'Označit jako odeslanou', 'Zamknout']) {
      expect(within(menu).queryByRole('menuitem', { name })).not.toBeInTheDocument()
    }
  })

  it('vlastník vidí úpravy a akce stavu', async () => {
    const inv = makeInvoice({ status: 'open' })
    mockInvoice(inv)
    const { user } = await openDetail(inv)
    expect(action('Upravit')).toHaveAttribute('href', '/a/firma/invoices/42/edit')
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    const menu = await screen.findByRole('menu')
    for (const name of ['Duplikovat', 'Označit jako odeslanou', 'Stornovat']) {
      expect(within(menu).getByRole('menuitem', { name })).toBeInTheDocument()
    }
  })

  it('mobil: PDF jako krátká akce, bez stažení v menu', async () => {
    setMobile()
    const inv = makeInvoice({ status: 'open' })
    mockInvoice(inv)
    const { user } = await openDetail(inv)
    expect(action('Otevřít PDF')).toBeNull()
    // primární akce jako ikony v liště
    expect(action('Přidat platbu')).toBeInTheDocument()
    expect(action('PDF')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    const menu = await screen.findByRole('dialog')
    expect(within(menu).queryByText('Stáhnout PDF')).not.toBeInTheDocument()
    expect(within(menu).getByText('Sdílet odkaz pro klienta')).toBeInTheDocument()
  })
})

describe('detail faktury — koncept', () => {
  const draft = makeInvoice({ status: 'draft', number: '', variable_symbol: '', public_token: 'tok' })

  it('nabízí vystavení a skrývá akce, které koncept nemá', async () => {
    mockInvoice(draft)
    const issued: string[] = []
    server.use(
      http.post(`${API}/api/accounts/:slug/invoices/:id/actions/:action`, ({ params }) => {
        issued.push(String(params.action))
        return HttpResponse.json({ ...draft, status: 'open', number: '2026-0007' })
      }),
    )
    const { user } = await openDetail(draft)
    expect(screen.getByRole('heading', { level: 1, name: 'Koncept' })).toBeInTheDocument()
    expect(screen.getByText('Koncept — zatím nevystaveno')).toBeInTheDocument()
    for (const name of ['Přidat platbu', 'Odeslat e-mailem']) expect(action(name)).toBeNull()
    expect(action('Upravit')).toBeInTheDocument()
    expect(action('Otevřít PDF')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Další akce' }))
    const menu = await screen.findByRole('menu')
    for (const name of ['Duplikovat', 'Smazat', 'Zamknout']) {
      expect(within(menu).getByRole('menuitem', { name })).toBeInTheDocument()
    }
    for (const name of ['Stáhnout ISDOC', 'Označit jako odeslanou', 'Stornovat', 'Označit jako nedobytnou', 'Vystavit opravný doklad', 'Kopírovat odkaz pro klienta']) {
      expect(within(menu).queryByRole('menuitem', { name })).not.toBeInTheDocument()
    }
    await user.keyboard('{Escape}')

    // hlavní akce v hlavičce i v upozornění
    expect(screen.getAllByRole('button', { name: 'Vystavit' })).toHaveLength(2)
    await user.click(screen.getAllByRole('button', { name: 'Vystavit' })[0]!)
    expect(await screen.findByRole('heading', { level: 1, name: '2026-0007' })).toBeInTheDocument()
    expect(issued).toEqual(['issue'])
    expect(screen.queryByText('Koncept — zatím nevystaveno')).not.toBeInTheDocument()
  })

  it('mobil: Vystavit jako primární akce', async () => {
    setMobile()
    mockInvoice(draft)
    await openDetail(draft)
    expect(screen.getAllByRole('button', { name: 'Vystavit' }).length).toBeGreaterThan(0)
    expect(action('Přidat platbu')).toBeNull()
  })
})

describe('detail faktury — EET', () => {
  const payment = { id: 5, invoice_id: 42, amount: 1_000_000, paid_on: '2026-03-02', note: '', created_at: '' }

  /** Odpověď na platbu s tržbou v daném stavu. */
  function mockPaymentWithSale(inv: Invoice, sale: Partial<ReturnType<typeof makeEETSale>>) {
    const bodies: CreatePaymentInput[] = []
    server.use(
      http.post(`${API}/api/accounts/:slug/invoices/:id/payments`, async ({ request }) => {
        bodies.push((await request.json()) as CreatePaymentInput)
        return HttpResponse.json(
          {
            invoice: { ...inv, status: 'paid', paid_amount: inv.total, remaining_amount: 0, payments: [payment], eet_sales: [makeEETSale({ payment_id: 5, ...sale })] },
            payment,
          },
          { status: 201 },
        )
      }),
    )
    return bodies
  }

  it('bez zapnuté evidence volbu nenabízí a pole neposílá', async () => {
    const inv = makeInvoice({ payment_method: 'cash' })
    const calls = mockInvoice(inv)
    server.use(http.get(`${API}/api/accounts/:slug/eet`, () => HttpResponse.json(makeEETSettings({ enabled: false }))))
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(within(dialog).queryByRole('switch', { name: /Kontaktní platba/ })).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    await waitFor(() => expect(calls.payments).toHaveLength(1))
    expect('eet' in calls.payments[0]!).toBe(false)
  })

  it('hotovost předvolí kontaktní platbu a pošle eet: true', async () => {
    const inv = makeInvoice({ payment_method: 'cash' })
    mockInvoice(inv)
    server.use(http.get(`${API}/api/accounts/:slug/eet`, () => HttpResponse.json(makeEETSettings())))
    const bodies = mockPaymentWithSale(inv, {})
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(await within(dialog).findByRole('switch', { name: /Kontaktní platba — zaevidovat do EET/ })).toBeChecked()
    expect(within(dialog).getByText(/převod na účet se neeviduje/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]!.eet).toBe(true)
    // zaevidovaná tržba = žádné upozornění navíc
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Přidat platbu' })).not.toBeInTheDocument())
    expect((await screen.findAllByText('Faktura je uhrazená')).length).toBeGreaterThan(0)
    expect(screen.queryByText(/do EET/)).not.toBeInTheDocument()
  })

  it('převod nechá volbu vypnutou a pošle eet: false; čekající tržba jen upozorní', async () => {
    const inv = makeInvoice({ payment_method: 'bank' })
    mockInvoice(inv)
    server.use(http.get(`${API}/api/accounts/:slug/eet`, () => HttpResponse.json(makeEETSettings())))
    const bodies = mockPaymentWithSale(inv, { status: 'pending', pok: undefined, confirmed_at: undefined })
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(await within(dialog).findByRole('switch', { name: /Kontaktní platba/ })).not.toBeChecked()
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]!.eet).toBe(false)
    // čekající tržba → neblokující upozornění
    expect(await screen.findByText('Tržba čeká na odeslání do EET')).toBeInTheDocument()
    expect(screen.getByText(/odešle se automaticky/)).toBeInTheDocument()
    expect(screen.queryByRole('dialog', { name: 'Přidat platbu' })).not.toBeInTheDocument()
  })

  it('odmítnutá tržba ukáže chybu Finanční správy', async () => {
    const inv = makeInvoice({ payment_method: 'card' })
    mockInvoice(inv)
    server.use(http.get(`${API}/api/accounts/:slug/eet`, () => HttpResponse.json(makeEETSettings())))
    mockPaymentWithSale(inv, { status: 'failed', pok: undefined, confirmed_at: undefined, error: 'Neplatné EIČ', error_code: 12 })
    const { user } = await openDetail(inv)
    await user.click(screen.getByRole('button', { name: 'Přidat platbu' }))
    const dialog = await screen.findByRole('dialog', { name: 'Přidat platbu' })
    expect(await within(dialog).findByRole('switch', { name: /Kontaktní platba/ })).toBeChecked()
    await user.click(within(dialog).getByRole('button', { name: 'Přidat platbu' }))
    expect(await screen.findByText('Platbu se nepodařilo zaevidovat do EET')).toBeInTheDocument()
    expect(screen.getByText('Neplatné EIČ')).toBeInTheDocument()
  })

  it('platby ukazují stav a POK, storna jsou v podseznamu, odmítnutou jde poslat znovu', async () => {
    const failed = makeEETSale({ id: 3, payment_id: 6, status: 'failed', pok: undefined, error: 'Chybný podpis', amount: 50_000 })
    const inv = makeInvoice({
      status: 'paid',
      paid_amount: 1_000_000,
      remaining_amount: 0,
      payments: [payment, { ...payment, id: 6, amount: 50_000 }],
      eet_sales: [
        makeEETSale({ id: 1, payment_id: 5 }),
        failed,
        makeEETSale({ id: 2, payment_id: undefined, status: 'cancelled', pok: 'pok-smazane-platby', amount: 20_000 }),
        makeEETSale({ id: 4, payment_id: undefined, cancels_id: 2, pok: 'pok-storna', amount: -20_000 }),
      ],
    })
    mockInvoice(inv)
    const retried: string[] = []
    server.use(
      http.post(`${API}/api/accounts/:slug/eet-sales/:id/actions/retry`, ({ params }) => {
        retried.push(String(params.id))
        return HttpResponse.json({ ...failed, status: 'confirmed', pok: 'pok-po-opakovani', error: undefined })
      }),
    )
    const { user } = await openDetail(inv)
    expect(screen.getByText('EET: Zaevidováno')).toBeInTheDocument()
    expect(screen.getByText('EET: Odmítnuto')).toBeInTheDocument()
    expect(screen.getByText('Chybný podpis')).toBeInTheDocument()
    expect(screen.getByText('EET — storna a smazané platby')).toBeInTheDocument()
    expect(screen.getByText('pok-storna')).toBeInTheDocument()
    expect(screen.getByText('pok-smazane-platby')).toBeInTheDocument()
    expect(screen.getByText(/storno tržby/)).toBeInTheDocument()
    expect(screen.getByText(/tržba smazané platby/)).toBeInTheDocument()

    expect(screen.getAllByRole('button', { name: 'Zkusit znovu' })).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Zkusit znovu' }))
    expect(await screen.findByText('Tržba je zaevidovaná')).toBeInTheDocument()
    expect(retried).toEqual(['3'])
  })

  it('smazání zaevidované platby upozorní na storno', async () => {
    const inv = makeInvoice({
      status: 'paid',
      paid_amount: 1_000_000,
      remaining_amount: 0,
      payments: [payment, { ...payment, id: 6 }],
      eet_sales: [makeEETSale({ payment_id: 5 })],
    })
    mockInvoice(inv)
    const { user } = await openDetail(inv)
    const [withSale, withoutSale] = screen.getAllByRole('button', { name: 'Smazat platbu' })
    await user.click(withoutSale!)
    let dialog = await screen.findByRole('dialog', { name: 'Smazat platbu?' })
    expect(within(dialog).queryByText(/storno/)).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Zpět' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Smazat platbu?' })).not.toBeInTheDocument())

    await user.click(withSale!)
    dialog = await screen.findByRole('dialog', { name: 'Smazat platbu?' })
    expect(within(dialog).getByText(/Finanční správě se automaticky odešle její storno/)).toBeInTheDocument()
  })

  it('účetní vidí stav tržby, ale ne nový pokus', async () => {
    setSession('accountant')
    const inv = makeInvoice({
      payments: [payment],
      eet_sales: [makeEETSale({ payment_id: 5, status: 'failed', pok: undefined, error: 'Chybný podpis' })],
    })
    mockInvoice(inv)
    await openDetail(inv)
    expect(screen.getByText('EET: Odmítnuto')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Zkusit znovu' })).not.toBeInTheDocument()
  })
})
