import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { EETCertificateInput, EETSale, EETSettings, UpdateEETSettingsInput } from '@/api/types'
import { makeEETSale, makeEETSettings } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'
import { setMobile } from '@/test/viewport'

function mockEet(initial: EETSettings, sales: EETSale[] = []) {
  const calls = {
    patches: [] as UpdateEETSettingsInput[],
    uploads: [] as EETCertificateInput[],
    deleted: 0,
    tests: 0,
    retried: [] as string[],
    saleQueries: [] as string[],
  }
  let settings = initial
  server.use(
    http.get(`${API}/api/accounts/:slug/eet`, () => HttpResponse.json(settings)),
    http.patch(`${API}/api/accounts/:slug/eet`, async ({ request }) => {
      const body = (await request.json()) as UpdateEETSettingsInput
      calls.patches.push(body)
      if (body.eic === 'CZ99999999') {
        return problem(422, 'validation failed', { errors: [{ location: 'body.eic', message: 'EIČ neexistuje' }] })
      }
      if (body.enabled && !settings.has_certificate) return problem(409, 'not configured', { code: 'eet_not_configured' })
      settings = { ...settings, ...body }
      return HttpResponse.json(settings)
    }),
    http.put(`${API}/api/accounts/:slug/eet/certificate`, async ({ request }) => {
      const body = (await request.json()) as EETCertificateInput
      calls.uploads.push(body)
      if (body.password !== 'tajne') return problem(422, 'bad certificate', { code: 'eet_bad_certificate' })
      settings = {
        ...settings,
        has_certificate: true,
        certificate_eic: 'CZ12345678',
        certificate_valid_to: '2099-01-01T00:00:00Z',
        eic: settings.eic || 'CZ12345678',
      }
      return HttpResponse.json(settings)
    }),
    http.delete(`${API}/api/accounts/:slug/eet/certificate`, () => {
      calls.deleted++
      settings = { ...settings, has_certificate: false, certificate_eic: undefined, certificate_valid_to: undefined, enabled: false }
      return new HttpResponse(null, { status: 204 })
    }),
    http.post(`${API}/api/accounts/:slug/eet/actions/test`, () => {
      calls.tests++
      return HttpResponse.json({
        ok: true,
        environment: settings.environment,
        code: 0,
        message: 'Zpráva byla zpracována',
        warnings: [{ code: 5, text: 'Certifikát brzy vyprší' }],
        transaction_id: 'tx-123',
      })
    }),
    http.get(`${API}/api/accounts/:slug/eet-sales`, ({ request }) => {
      const q = new URL(request.url).searchParams
      calls.saleQueries.push(q.get('status') ?? '')
      const items = sales.filter((s) => !q.get('status') || s.status === q.get('status'))
      return HttpResponse.json({ items, total: items.length, page: 1, per_page: 25 })
    }),
    http.post(`${API}/api/accounts/:slug/eet-sales/:id/actions/retry`, ({ params }) => {
      calls.retried.push(String(params.id))
      const sale = sales.find((s) => String(s.id) === params.id)!
      return HttpResponse.json({ ...sale, status: 'confirmed', pok: 'pok-novy', error: undefined })
    }),
  )
  return calls
}

async function openPage(search = '') {
  const app = await renderApp(`/a/firma/settings/eet${search}`)
  await screen.findByRole('heading', { level: 2, name: 'Evidence tržeb' })
  return app
}

const unconfigured = makeEETSettings({
  enabled: false,
  eic: '',
  unit_id: '',
  register_id: '',
  has_certificate: false,
  certificate_eic: undefined,
  certificate_valid_to: undefined,
})

describe('nastavení EET', () => {
  it('je v menu nastavení a vysvětluje, co se eviduje', async () => {
    mockEet(makeEETSettings())
    await openPage()
    expect(screen.getByRole('link', { name: 'EET' })).toHaveAttribute('href', '/a/firma/settings/eet')
    expect(screen.getByText(/Úhrada faktury převodem na dálku se neeviduje/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'eet.gov.cz' })).toHaveAttribute('href', 'https://eet.gov.cz')
  })

  it('nenastavený účet: zapnutí je zamčené, EIČ jde převzít z DIČ a údaje uložit', async () => {
    setSession('owner', { vat_no: 'CZ12345678' })
    const calls = mockEet(unconfigured)
    const { user } = await openPage()
    expect(screen.getByRole('switch', { name: 'Evidovat kontaktní platby' })).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Otestovat spojení' })).toBeDisabled()
    expect(screen.getByText('Certifikát zatím není nahraný')).toBeInTheDocument()
    expect(screen.getByLabelText('Označení pokladny')).toHaveValue('1')

    // validace před odesláním
    await user.click(screen.getByRole('button', { name: 'Uložit údaje' }))
    expect(await screen.findByText(/EIČ má tvar CZ a 8–10 číslic/)).toBeInTheDocument()
    expect(screen.getByText(/Číslo evidenční jednotky je 1–9 číslic/)).toBeInTheDocument()
    expect(calls.patches).toHaveLength(0)

    await user.click(await screen.findByRole('button', { name: 'Použít DIČ firmy CZ12345678' }))
    expect(screen.getByLabelText('EIČ')).toHaveValue('CZ12345678')
    await user.type(screen.getByLabelText('Číslo evidenční jednotky'), '11')
    await user.click(screen.getByRole('button', { name: 'Uložit údaje' }))
    expect(await screen.findByText('Údaje pro evidenci uloženy')).toBeInTheDocument()
    expect(calls.patches).toEqual([{ eic: 'CZ12345678', unit_id: '11', register_id: '1' }])
  })

  it('chybu 422 ze serveru ukáže u pole', async () => {
    mockEet(makeEETSettings())
    const { user } = await openPage()
    const eic = screen.getByLabelText('EIČ')
    await user.clear(eic)
    await user.type(eic, 'CZ99999999')
    await user.click(screen.getByRole('button', { name: 'Uložit údaje' }))
    expect(await screen.findByText('EIČ neexistuje')).toBeInTheDocument()
  })

  it('nahraje certifikát jako base64 s heslem; špatné heslo ukáže chybu v dialogu', async () => {
    const calls = mockEet({ ...unconfigured, eic: 'CZ12345678', unit_id: '11', register_id: '1' })
    const { user } = await openPage()
    await user.click(screen.getByRole('button', { name: 'Nahrát certifikát' }))
    const dialog = await screen.findByRole('dialog', { name: 'Nahrát pokladní certifikát' })

    await user.click(within(dialog).getByRole('button', { name: 'Nahrát certifikát' }))
    expect(await within(dialog).findByText('Vyberte soubor s certifikátem (.p12).')).toBeInTheDocument()

    const file = new File([new Uint8Array([0x30, 0x82, 0xff, 0x00])], 'pokladna.p12', { type: 'application/x-pkcs12' })
    await user.upload(within(dialog).getByLabelText('Soubor certifikátu (.p12)'), file)
    expect(within(dialog).getByText('pokladna.p12')).toBeInTheDocument()
    await user.type(within(dialog).getByLabelText('Heslo k certifikátu'), 'spatne')
    await user.click(within(dialog).getByRole('button', { name: 'Nahrát certifikát' }))
    expect(await within(dialog).findByText('Soubor není pokladní certifikát (.p12), nebo nesedí heslo.')).toBeInTheDocument()

    await user.clear(within(dialog).getByLabelText('Heslo k certifikátu'))
    await user.type(within(dialog).getByLabelText('Heslo k certifikátu'), 'tajne')
    await user.click(within(dialog).getByRole('button', { name: 'Nahrát certifikát' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Nahrát pokladní certifikát' })).not.toBeInTheDocument())
    expect(calls.uploads).toEqual([
      { pkcs12: 'MIL/AA==', password: 'spatne' },
      { pkcs12: 'MIL/AA==', password: 'tajne' },
    ])
    expect(await screen.findByText('Certifikát je nahraný')).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Evidovat kontaktní platby' })).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('zapne evidenci, přepne prostředí a varuje u testovacího', async () => {
    const calls = mockEet(makeEETSettings({ enabled: false }))
    const { user } = await openPage()
    await user.click(screen.getByRole('switch', { name: 'Evidovat kontaktní platby' }))
    expect(await screen.findByText('Evidence tržeb zapnuta')).toBeInTheDocument()
    expect(screen.queryByText(/Testovací prostředí — tržby se ve skutečnosti neevidují/)).not.toBeInTheDocument()

    await user.click(screen.getByRole('combobox', { name: 'Prostředí' }))
    await user.click(await screen.findByRole('option', { name: 'Testovací prostředí' }))
    expect(await screen.findByText(/Testovací prostředí — tržby se ve skutečnosti neevidují/)).toBeInTheDocument()
    expect(screen.getByText(/potvrzovací kód končí „-ff“/)).toBeInTheDocument()
    expect(calls.patches).toEqual([{ enabled: true }, { environment: 'playground' }])
  })

  it('varuje před vypršením certifikátu a jiným EIČ; odebrání vypne evidenci', async () => {
    const soon = new Date(Date.now() + 5 * 24 * 60 * 60 * 1000 + 60_000).toISOString()
    const calls = mockEet(makeEETSettings({ certificate_valid_to: soon, certificate_eic: 'CZ87654321' }))
    const { user } = await openPage()
    expect(screen.getByText(/Platnost certifikátu vyprší za 5 dní/)).toBeInTheDocument()
    expect(screen.getByText(/Certifikát je vystaven pro EIČ CZ87654321/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Odebrat' }))
    const dialog = await screen.findByRole('dialog', { name: 'Odebrat certifikát?' })
    expect(within(dialog).getByText(/evidence tržeb se vypne/)).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Odebrat certifikát' }))
    expect(await screen.findByText('Certifikát zatím není nahraný')).toBeInTheDocument()
    expect(calls.deleted).toBe(1)
    expect(screen.getByRole('switch', { name: 'Evidovat kontaktní platby' })).not.toBeChecked()
  })

  it('test spojení ukáže výsledek, varování a ID transakce', async () => {
    const calls = mockEet(makeEETSettings())
    const { user } = await openPage()
    await user.click(screen.getByRole('button', { name: 'Otestovat spojení' }))
    expect(await screen.findByText('Spojení funguje')).toBeInTheDocument()
    expect(screen.getByText('Zpráva byla zpracována')).toBeInTheDocument()
    expect(screen.getByText('Certifikát brzy vyprší')).toBeInTheDocument()
    expect(screen.getByText('tx-123')).toBeInTheDocument()
    expect(calls.tests).toBe(1)
  })

  it('seznam tržeb: stav, POK, chyba, filtr a nový pokus', async () => {
    const sales = [
      makeEETSale({ id: 1 }),
      makeEETSale({ id: 2, status: 'failed', pok: undefined, error: 'Neplatný podpis', error_code: 4, invoice_number: '2026-0002', invoice_id: 43 }),
      makeEETSale({ id: 3, cancels_id: 1, amount: -1_000_000, pok: 'pok-storno' }),
    ]
    const calls = mockEet(makeEETSettings({ failed_count: 1, pending_count: 2 }), sales)
    const { user, router } = await openPage()
    expect(screen.getByText(/^1 tržbu Finanční správa odmítla/)).toBeInTheDocument()
    expect(screen.getByText(/^2 tržby čekají na odeslání/)).toBeInTheDocument()

    // desktop: tabulka (mobilní karty jsou skryté)
    const table = within(await screen.findByRole('table'))
    expect(table.getByText('a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d')).toBeInTheDocument()
    expect(table.getByText('Storno')).toBeInTheDocument()
    expect(table.getByText('Neplatný podpis (kód 4)')).toBeInTheDocument()
    expect(table.getByRole('link', { name: '2026-0002' })).toHaveAttribute('href', '/a/firma/invoices/43')
    expect(table.getAllByRole('button', { name: 'Kopírovat potvrzovací kód' })).toHaveLength(2)

    await user.click(screen.getByRole('button', { name: 'Zobrazit odmítnuté tržby' }))
    await waitFor(() => expect(router.state.location.search).toEqual({ status: 'failed' }))
    await waitFor(() => expect(screen.queryAllByText('pok-storno')).toHaveLength(0))
    expect(calls.saleQueries).toContain('failed')
    expect(screen.getByRole('button', { name: 'Odmítnuto', pressed: true })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Zkusit znovu' }))
    expect(await screen.findByText('Tržba je zaevidovaná')).toBeInTheDocument()
    expect(calls.retried).toEqual(['2'])
  })

  it('mobil: tržby jako karty s akcí přes celou šířku', async () => {
    setMobile()
    mockEet(makeEETSettings(), [makeEETSale({ id: 2, status: 'pending', pok: undefined, deadline_at: '2099-01-01T00:00:00Z' })])
    await openPage()
    expect(await screen.findByRole('button', { name: 'Zkusit znovu' })).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.getAllByText(/Zaevidovat do/).length).toBeGreaterThan(0)
  })

  it('člen bez práva na nastavení vidí stav, ale nemůže měnit', async () => {
    setSession('member')
    mockEet(makeEETSettings(), [makeEETSale({ id: 2, status: 'failed', pok: undefined, error: 'Chyba' })])
    await openPage()
    expect(screen.getByText('Nastavení EET může měnit jen vlastník nebo administrátor účtu.')).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Evidovat kontaktní platby' })).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByLabelText('EIČ')).toBeDisabled()
    expect(screen.getByText('Certifikát je nahraný')).toBeInTheDocument()
    for (const name of ['Uložit údaje', 'Nahradit certifikát', 'Odebrat', 'Otestovat spojení']) {
      expect(screen.queryByRole('button', { name })).not.toBeInTheDocument()
    }
    // člen smí tržbu poslat znovu, účetní ne (viz detail faktury)
    expect(await screen.findByRole('button', { name: 'Zkusit znovu' })).toBeInTheDocument()
  })
})
