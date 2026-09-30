import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { BankTransaction } from '@/api/types'
import { makeBankAccount, makeInvoice, TS } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'
import { setMobile } from '@/test/viewport'

function tx(overrides: Partial<BankTransaction>): BankTransaction {
  return {
    id: 1,
    bank_account_id: 3,
    amount: 1_000_000,
    currency: 'CZK',
    booked_on: '2026-03-10',
    counterparty_name: 'ACME a.s.',
    counterparty_account: '19-2000145399/0800',
    message: '',
    variable_symbol: '20260001',
    constant_symbol: '',
    specific_symbol: '',
    external_id: 'x1',
    auto_matched: false,
    ignored: false,
    state: 'unmatched',
    suggestions: [],
    created_at: TS,
    ...overrides,
  }
}

/** Stavový backend banky: párování mění stav transakce, seznam filtruje podle `state`. */
function mockBank(initial: BankTransaction[]) {
  const txs = new Map(initial.map((t) => [t.id, t]))
  const calls = { match: [] as { id: number; body: unknown }[], unmatch: [] as number[], ignore: [] as number[] }
  const invoice = makeInvoice({ id: 50, number: '2026-0050', client_name: 'ACME a.s.', remaining_amount: 1_000_000, total: 1_000_000 })
  server.use(
    http.get(`${API}/api/accounts/:slug/bank-accounts`, () =>
      HttpResponse.json({ items: [makeBankAccount()], total: 1, page: 1, per_page: 200 }),
    ),
    http.get(`${API}/api/accounts/:slug/bank-transactions`, ({ request }) => {
      const q = new URL(request.url).searchParams
      const state = q.get('state')
      const items = [...txs.values()].filter((t) => !state || t.state === state)
      const perPage = Number(q.get('per_page') ?? 30)
      return HttpResponse.json({ items: items.slice(0, perPage), total: items.length, page: 1, per_page: perPage })
    }),
    http.get(`${API}/api/accounts/:slug/invoices`, () =>
      HttpResponse.json({ items: [invoice], total: 1, page: 1, per_page: 50, sums: [] }),
    ),
    http.post(`${API}/api/accounts/:slug/bank-transactions/:id/match`, async ({ params, request }) => {
      const id = Number(params.id)
      const body = (await request.json()) as { invoice_id?: number }
      calls.match.push({ id, body })
      if (body.invoice_id === 666) return problem(409, 'already paid', { code: 'conflict' })
      const t = { ...txs.get(id)!, state: 'matched' as const, matched_invoice_id: body.invoice_id, matched_number: '2026-0050', suggestions: [] }
      txs.set(id, t)
      return HttpResponse.json(t)
    }),
    http.post(`${API}/api/accounts/:slug/bank-transactions/:id/unmatch`, ({ params }) => {
      const id = Number(params.id)
      calls.unmatch.push(id)
      const t = { ...txs.get(id)!, state: 'unmatched' as const, matched_invoice_id: undefined, matched_number: undefined }
      txs.set(id, t)
      return HttpResponse.json(t)
    }),
    http.post(`${API}/api/accounts/:slug/bank-transactions/:id/ignore`, ({ params }) => {
      const id = Number(params.id)
      calls.ignore.push(id)
      const t = { ...txs.get(id)!, state: 'ignored' as const, ignored: true }
      txs.set(id, t)
      return HttpResponse.json(t)
    }),
  )
  return calls
}

const suggested = tx({
  id: 1,
  state: 'suggested',
  suggestions: [
    { invoice_id: 50, number: '2026-0050', name: 'ACME a.s.', remaining: 1_000_000, score: 90, reasons: ['vs', 'amount'] },
    { invoice_id: 51, number: '2026-0051', name: 'Jiný', remaining: 500_000, score: 40, reasons: ['amount'] },
  ],
})

async function openBank(url = '/a/firma/bank?tab=all') {
  const app = await renderApp(url)
  await screen.findByRole('heading', { level: 1, name: 'Banka' })
  return app
}

describe('banka — párování plateb', () => {
  it('návrh spáruje jedním klepnutím a toast nabídne „Vrátit“', async () => {
    const calls = mockBank([suggested])
    const { user } = await openBank()
    const first = (await screen.findAllByRole('button', { name: 'Spárovat' }))[0]!
    await user.click(first)
    await waitFor(() => expect(calls.match).toEqual([{ id: 1, body: { invoice_id: 50 } }]))
    expect(await screen.findByText('Spárováno: faktura 2026-0050')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Vrátit' }))
    await waitFor(() => expect(calls.unmatch).toEqual([1]))
  })

  it('ruční spárování přes hledání dokladu', async () => {
    const calls = mockBank([tx({ id: 2, variable_symbol: '' })])
    const { user } = await openBank()
    await user.click(await screen.findByRole('button', { name: 'Hledat doklad…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Spárovat s dokladem' })
    // stejná částka → „Částka sedí“
    const candidate = await within(dialog).findByRole('button', { name: /2026-0050/ })
    expect(within(candidate).getByText('Částka sedí')).toBeInTheDocument()
    await user.click(candidate)
    await waitFor(() => expect(calls.match).toEqual([{ id: 2, body: { invoice_id: 50 } }]))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Spárovat s dokladem' })).not.toBeInTheDocument())
  })

  it('zrušení spárování vyžaduje potvrzení', async () => {
    const calls = mockBank([tx({ id: 3, state: 'matched', matched_invoice_id: 50, matched_number: '2026-0050', matched_name: 'ACME a.s.' })])
    const { user } = await openBank()
    await user.click(await screen.findByRole('button', { name: 'Zrušit spárování' }))
    const dialog = await screen.findByRole('alertdialog', { name: 'Zrušit spárování?' }).catch(() => screen.findByRole('dialog', { name: 'Zrušit spárování?' }))
    await user.click(within(dialog).getByRole('button', { name: 'Ponechat' }))
    expect(calls.unmatch).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Zrušit spárování' }))
    const again = await screen.findByRole('alertdialog', { name: 'Zrušit spárování?' }).catch(() => screen.findByRole('dialog', { name: 'Zrušit spárování?' }))
    await user.click(within(again).getByRole('button', { name: 'Zrušit spárování' }))
    await waitFor(() => expect(calls.unmatch).toEqual([3]))
    expect(await screen.findByText('Spárování zrušeno — úhrada u dokladu byla smazána')).toBeInTheDocument()
  })

  it('ignorování platby', async () => {
    const calls = mockBank([tx({ id: 4 })])
    const { user } = await openBank()
    await user.click(await screen.findByRole('button', { name: 'Ignorovat' }))
    await waitFor(() => expect(calls.ignore).toEqual([4]))
    expect(await screen.findByText('Platba ignorována')).toBeInTheDocument()
  })

  it('účetní vidí návrhy, ale nemůže párovat ani importovat', async () => {
    setSession('accountant')
    mockBank([suggested])
    await openBank()
    expect(await screen.findByText('2026-0051')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Spárovat' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Hledat doklad…' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Importovat výpis' })).not.toBeInTheDocument()
  })

  it('mobil: karty s akcemi párování', async () => {
    setMobile()
    const calls = mockBank([suggested])
    const { user } = await openBank()
    await user.click((await screen.findAllByRole('button', { name: 'Spárovat' }))[0]!)
    await waitFor(() => expect(calls.match).toHaveLength(1))
  })
})
