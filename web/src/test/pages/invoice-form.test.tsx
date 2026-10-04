import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { CreateInvoiceInput } from '@/api/types'
import { formatMoney, formatVatRate } from '@/lib/money'
import { makeBankAccount, makeInvoice, makeSubject } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'
import { setMobile } from '@/test/viewport'

// Testing Library porovnává text s normalizovanými mezerami → nezlomitelné mezery z Intl nahradíme.
const czk = (minor: number) => formatMoney(minor, 'CZK').replace(/\s/g, ' ')
const rate = (bps: number) => formatVatRate(bps).replace(/\s/g, ' ')

function useInvoiceApi() {
  const created: CreateInvoiceInput[] = []
  server.use(
    http.get(`${API}/api/accounts/:slug/bank-accounts`, () =>
      HttpResponse.json({ items: [makeBankAccount()], total: 1, page: 1, per_page: 200 }),
    ),
    http.get(`${API}/api/accounts/:slug/subjects`, () =>
      HttpResponse.json({ items: [makeSubject()], total: 1, page: 1, per_page: 30 }),
    ),
    http.post(`${API}/api/accounts/:slug/invoices`, async ({ request }) => {
      created.push((await request.json()) as CreateInvoiceInput)
      return HttpResponse.json(makeInvoice(), { status: 201 })
    }),
    http.get(`${API}/api/accounts/:slug/invoices/:id`, () => HttpResponse.json(makeInvoice())),
  )
  return created
}

async function openNewInvoice() {
  const app = await renderApp('/a/firma/invoices/new')
  await screen.findByRole('heading', { name: 'Položky' })
  return app
}

/** Rekapitulace DPH jako [sazba, základ, DPH] po řádcích. */
function vatRecapRows() {
  const table = screen.getByRole('table', { name: 'Rekapitulace DPH' })
  return within(table)
    .getAllByRole('row')
    .slice(1)
    .map((r) => [within(r).getByRole('rowheader'), ...within(r).getAllByRole('cell')].map((c) => c.textContent?.replace(/\s/g, ' ')))
}

async function pickVat(user: ReturnType<typeof import('@testing-library/user-event').default.setup>, index: number, bps: number) {
  await user.click(screen.getAllByRole('combobox', { name: 'Sazba DPH' })[index]!)
  await user.click(await screen.findByRole('option', { name: formatVatRate(bps) }))
}

describe('formulář faktury (desktop)', () => {
  it('editor položek: přidání, přesun, odebrání a živé součty s více sazbami DPH', async () => {
    setSession('owner', { vat_mode: 'vat_payer', vat_no: 'CZ12345678' })
    useInvoiceApi()
    const { user } = await openNewInvoice()

    await user.type(screen.getByLabelText('Položka 1: název'), 'Vývoj')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '10000')
    // Enter na posledním řádku přidá nový a zaměří jeho název
    await user.keyboard('{Enter}')
    expect(screen.getByLabelText('Položka 2: název')).toHaveFocus()
    await user.keyboard('Hosting')
    await user.clear(screen.getByLabelText('Položka 2: množství'))
    await user.type(screen.getByLabelText('Položka 2: množství'), '2')
    await user.type(screen.getByLabelText('Položka 2: cena za jednotku'), '1000')
    await pickVat(user, 1, 1200)

    await waitFor(() =>
      expect(vatRecapRows()).toEqual([
        [rate(2100), czk(1_000_000), czk(210_000)],
        [rate(1200), czk(200_000), czk(24_000)],
      ]),
    )
    expect(screen.getAllByText(czk(1_434_000)).length).toBeGreaterThan(0)

    // přesun: Alt+↑ v řádku 2 → Hosting je první
    await user.click(screen.getByLabelText('Položka 2: název'))
    await user.keyboard('{Alt>}{ArrowUp}{/Alt}')
    expect(screen.getByLabelText('Položka 1: název')).toHaveValue('Hosting')
    expect(screen.getByLabelText('Položka 2: název')).toHaveValue('Vývoj')
    // šipka na úchytu přesune zpět
    await user.click(screen.getByRole('button', { name: /Položka 1: přesunout/ }))
    await user.keyboard('{ArrowDown}')
    expect(screen.getByLabelText('Položka 1: název')).toHaveValue('Vývoj')

    // odebrání: sazba 12 % zmizí z rekapitulace; poslední řádek odebrat nejde
    await user.click(screen.getByRole('button', { name: 'Odebrat položku 2' }))
    await waitFor(() => expect(vatRecapRows()).toEqual([[rate(2100), czk(1_000_000), czk(210_000)]]))
    expect(screen.getByRole('button', { name: 'Odebrat položku 1' })).toBeDisabled()
  })

  it('ceny včetně DPH a přenesená daňová povinnost přepočítají součty', async () => {
    setSession('owner', { vat_mode: 'vat_payer', vat_no: 'CZ12345678' })
    useInvoiceApi()
    const { user } = await openNewInvoice()
    await user.type(screen.getByLabelText('Položka 1: název'), 'Vývoj')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '12100')
    await waitFor(() => expect(vatRecapRows()).toEqual([[rate(2100), czk(1_210_000), czk(254_100)]]))

    await user.click(screen.getByRole('switch', { name: 'Ceny včetně DPH' }))
    await waitFor(() => expect(vatRecapRows()).toEqual([[rate(2100), czk(1_000_000), czk(210_000)]]))
    expect(screen.getByRole('columnheader', { name: 'Celkem s DPH' })).toBeInTheDocument()

    await user.click(screen.getByRole('switch', { name: 'Ceny včetně DPH' }))
    await user.click(screen.getByRole('switch', { name: 'Přenesená daňová povinnost' }))
    expect(await screen.findByText('DPH (přenesená povinnost)')).toBeInTheDocument()
    // DPH odvede odběratel → celkem = základ
    await waitFor(() => expect(screen.getAllByText(czk(1_210_000)).length).toBeGreaterThan(1))
  })

  it('neplátce: bez sazeb DPH, zaokrouhlení celkem', async () => {
    setSession('owner', { vat_mode: 'non_vat_payer' })
    useInvoiceApi()
    const { user } = await openNewInvoice()
    expect(screen.queryByRole('combobox', { name: 'Sazba DPH' })).not.toBeInTheDocument()
    expect(screen.queryByRole('switch', { name: 'Ceny včetně DPH' })).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Položka 1: název'), 'Konzultace')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '99,60')
    await user.click(screen.getByRole('switch', { name: 'Zaokrouhlit celkem' }))
    expect(await screen.findByText('Zaokrouhlení')).toBeInTheDocument()
    expect(screen.getAllByText(czk(10_000)).length).toBeGreaterThan(0)
  })

  it('odešle správné tělo a přejde na detail', async () => {
    setSession('owner', { vat_mode: 'vat_payer', vat_no: 'CZ12345678' })
    const created = useInvoiceApi()
    const { user, router } = await openNewInvoice()

    await user.click(screen.getByRole('button', { name: 'Odběratel' }))
    await user.click(await screen.findByRole('option', { name: /ACME a\.s\./ }))
    await user.type(screen.getByLabelText('Položka 1: název'), 'Vývoj')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '1 234,50')
    await user.click(screen.getByRole('button', { name: 'Vystavit' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/42'))
    expect(created).toHaveLength(1)
    expect(created[0]).toMatchObject({
      subject_id: 7,
      document_type: 'invoice',
      lines: [{ name: 'Vývoj', quantity: '1', unit_price: 123_450, vat_rate_bps: 2100 }],
    })
  })

  it('koncový zákazník: bez kontaktu, jméno a e-mail přímo na faktuře', async () => {
    setSession('owner')
    const created = useInvoiceApi()
    const { user, router } = await openNewInvoice()

    await user.click(screen.getByRole('switch', { name: 'Koncový zákazník' }))
    expect(screen.queryByRole('button', { name: 'Odběratel' })).not.toBeInTheDocument()
    const name = screen.getByLabelText('Jméno')
    expect(name).toHaveValue('Koncový zákazník')
    await user.clear(name)
    await user.type(name, 'Jan Novák')
    await user.type(screen.getByLabelText('E-mail (nepovinné)'), 'jan@example.cz')
    await user.type(screen.getByLabelText('Položka 1: název'), 'Oprava kola')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '1 500')
    await user.click(screen.getByRole('button', { name: 'Vystavit' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/42'))
    expect(created).toHaveLength(1)
    expect(created[0]).toMatchObject({ client_name: 'Jan Novák', client_email: 'jan@example.cz' })
    expect(created[0]!.subject_id).toBeUndefined()
  })

  it('uloží koncept (draft: true) a přejde na detail', async () => {
    setSession('owner')
    const created = useInvoiceApi()
    const { user, router } = await openNewInvoice()
    await user.click(screen.getByRole('button', { name: 'Odběratel' }))
    await user.click(await screen.findByRole('option', { name: /ACME a\.s\./ }))
    await user.type(screen.getByLabelText('Položka 1: název'), 'Vývoj')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '100')
    await user.click(screen.getByRole('button', { name: 'Uložit jako koncept' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/42'))
    expect(created).toHaveLength(1)
    expect(created[0]).toMatchObject({ subject_id: 7, draft: true })
  })

  it('úprava konceptu: „Uložit a vystavit“ zavolá vystavení', async () => {
    setSession('owner')
    useInvoiceApi()
    const draft = makeInvoice({ status: 'draft', number: '', variable_symbol: '' })
    const issued: string[] = []
    server.use(
      http.get(`${API}/api/accounts/:slug/invoices/:id`, () => HttpResponse.json(draft)),
      http.post(`${API}/api/accounts/:slug/invoices/:id/actions/:action`, ({ params }) => {
        issued.push(String(params.action))
        return HttpResponse.json({ ...draft, status: 'open', number: '2026-0007' })
      }),
    )
    const { user, router } = await renderApp('/a/firma/invoices/42/edit')
    expect(await screen.findByRole('heading', { level: 1, name: 'Upravit koncept' })).toBeInTheDocument()
    // bez změn jde koncept rovnou vystavit, „Uložit koncept“ čeká na změnu
    expect(screen.getByRole('button', { name: 'Uložit koncept' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Uložit a vystavit' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/42'))
    expect(issued).toEqual(['issue'])
  })

  it('chybu 422 z API promítne do polí', async () => {
    setSession('owner')
    useInvoiceApi()
    server.use(
      http.post(`${API}/api/accounts/:slug/invoices`, () =>
        problem(422, 'validation failed', {
          errors: [
            { location: 'body.subject_id', message: 'odběratel neexistuje' },
            { location: 'body.lines[0].name', message: 'název je povinný na serveru' },
          ],
        }),
      ),
    )
    const { user, router } = await openNewInvoice()
    await user.click(screen.getByRole('button', { name: 'Odběratel' }))
    await user.click(await screen.findByRole('option', { name: /ACME/ }))
    await user.type(screen.getByLabelText('Položka 1: název'), 'X')
    await user.type(screen.getByLabelText('Položka 1: cena za jednotku'), '100')
    await user.click(screen.getByRole('button', { name: 'Vystavit' }))

    expect(await screen.findByText('odběratel neexistuje')).toBeInTheDocument()
    expect(screen.getByText('název je povinný na serveru')).toBeInTheDocument()
    expect(screen.getByLabelText('Položka 1: název')).toHaveAttribute('aria-invalid', 'true')
    expect(router.state.location.pathname).toBe('/a/firma/invoices/new')
  })

  it('klientská validace: bez odběratele a názvu položky nic neodešle', async () => {
    setSession('owner')
    const created = useInvoiceApi()
    const { user } = await openNewInvoice()
    await user.click(screen.getByRole('button', { name: 'Vystavit' }))
    await waitFor(() => expect(screen.getByLabelText('Položka 1: název')).toHaveAttribute('aria-invalid', 'true'))
    expect(screen.getByRole('button', { name: 'Odběratel' })).toHaveAttribute('aria-invalid', 'true')
    expect(created).toHaveLength(0)
  })

  it('neuložené změny: odchod se potvrzuje, Zůstat ponechá formulář', async () => {
    setSession('owner')
    useInvoiceApi()
    server.use(http.get(`${API}/api/accounts/:slug/invoices`, () => HttpResponse.json({ items: [], total: 0, page: 1, per_page: 50, sums: [] })))
    const { user, router } = await openNewInvoice()
    await user.type(screen.getByLabelText('Položka 1: název'), 'Rozepsáno')

    await user.click(screen.getAllByRole('link', { name: 'Kontakty' })[0]!)
    const dialog = await screen.findByRole('dialog', { name: 'Zahodit neuložené změny?' })
    await user.click(within(dialog).getByRole('button', { name: 'Zůstat' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Zahodit neuložené změny?' })).not.toBeInTheDocument())
    expect(router.state.location.pathname).toBe('/a/firma/invoices/new')
    expect(screen.getByLabelText('Položka 1: název')).toHaveValue('Rozepsáno')

    await user.click(screen.getAllByRole('link', { name: 'Kontakty' })[0]!)
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Zahodit a odejít' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/subjects'))
  })

  it('čistý formulář odejít nechá bez dotazu', async () => {
    setSession('owner')
    useInvoiceApi()
    const { user, router } = await openNewInvoice()
    await user.click(screen.getAllByRole('link', { name: 'Kontakty' })[0]!)
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/subjects'))
    expect(screen.queryByRole('dialog', { name: 'Zahodit neuložené změny?' })).not.toBeInTheDocument()
  })

  it('účetní vidí jen upozornění „Jen pro čtení“', async () => {
    setSession('accountant')
    useInvoiceApi()
    await renderApp('/a/firma/invoices/new')
    expect(await screen.findByText('Jen pro čtení')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Vystavit' })).not.toBeInTheDocument()
  })
})

describe('formulář faktury (mobil)', () => {
  it('položky jako karty: přidání, posun a odebrání', async () => {
    setMobile()
    setSession('owner', { vat_mode: 'vat_payer', vat_no: 'CZ12345678' })
    useInvoiceApi()
    const { user } = await openNewInvoice()
    // na mobilu není tabulka, ale karty s popisky
    expect(screen.queryByRole('table', { name: 'Položky faktury' })).not.toBeInTheDocument()
    await user.type(screen.getByLabelText('Název položky'), 'První')
    await user.click(screen.getByRole('button', { name: 'Přidat položku' }))
    const names = () => screen.getAllByLabelText('Název položky')
    await waitFor(() => expect(names()).toHaveLength(2))
    await user.type(names()[1]!, 'Druhá')

    await user.click(screen.getAllByRole('button', { name: 'Posunout dolů' })[0]!)
    expect(names().map((i) => (i as HTMLInputElement).value)).toEqual(['Druhá', 'První'])
    const firstCard = names()[0]!.closest('li')!
    expect(within(firstCard).getByRole('button', { name: 'Posunout nahoru' })).toBeDisabled()

    await user.click(screen.getAllByRole('button', { name: 'Odebrat položku' })[0]!)
    await waitFor(() => expect(names()).toHaveLength(1))
    expect(names()[0]).toHaveValue('První')
  })
})
