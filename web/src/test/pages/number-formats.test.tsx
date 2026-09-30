import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CreateNumberFormatInput, NumberFormat } from '@/api/types'
import { TS } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'

const fmt = (o: Partial<NumberFormat>): NumberFormat => ({
  id: 1,
  document_type: 'invoice',
  format: '{YYYY}-{NNNN}',
  is_default: true,
  created_at: TS,
  updated_at: TS,
  ...o,
})

function mockFormats(formats: NumberFormat[]) {
  const created: CreateNumberFormatInput[] = []
  server.use(
    http.get(`${API}/api/accounts/:slug/number-formats`, () =>
      HttpResponse.json({ items: formats, total: formats.length, page: 1, per_page: 200 }),
    ),
    http.get(`${API}/api/accounts/:slug/number-formats/:id/preview`, ({ params }) =>
      HttpResponse.json({ number: params.id === '1' ? '2026-0042' : 'X' }),
    ),
    http.post(`${API}/api/accounts/:slug/number-formats`, async ({ request }) => {
      const body = (await request.json()) as CreateNumberFormatInput
      created.push(body)
      if (body.format === 'FV{YY}{NNNN}') return problem(409, 'exists', { code: 'already_exists' })
      return HttpResponse.json(fmt({ id: 9, ...body, is_default: Boolean(body.is_default) }), { status: 201 })
    }),
  )
  return created
}

beforeEach(() => {
  // náhled čísla se počítá z dnešního data
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date('2026-03-15T12:00:00'))
})
afterEach(() => vi.useRealTimers())

async function openPage() {
  const app = await renderApp('/a/firma/settings/number-formats')
  await screen.findByRole('heading', { level: 2, name: 'Faktury' })
  return app
}

describe('číselné řady', () => {
  it('seznam ukazuje příští číslo ze serveru a období nulování', async () => {
    mockFormats([fmt({})])
    await openPage()
    expect(await screen.findByText('2026-0042')).toBeInTheDocument()
    expect(screen.getByText('Číslování začíná každý rok znovu od 1.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Smazat řadu {YYYY}-{NNNN}' })).toBeDisabled()
  })

  it('editor: živý náhled, vkládání symbolů na kurzor, validace', async () => {
    const created = mockFormats([fmt({})])
    const { user } = await openPage()
    const proformas = screen.getByRole('region', { name: 'Zálohové faktury' })
    await user.click(within(proformas).getByRole('button', { name: /Přidat/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Nová řada · Zálohové faktury' })
    const input = within(dialog).getByLabelText('Formát')

    // návrh pro zálohy
    expect(input).toHaveValue('ZF{YY}{NNNN}')
    expect(within(dialog).getByText('ZF260001')).toBeInTheDocument()

    await user.clear(input)
    await user.type(input, 'Z-')
    await user.click(within(dialog).getByRole('button', { name: /\{MM\}/ }))
    expect(input).toHaveValue('Z-{MM}')
    // bez pořadového čísla → chyba místo náhledu
    expect(within(dialog).getByText('Formát musí obsahovat pořadové číslo {N}…{NNNNNN}')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Přidat řadu' }))
    expect(created).toHaveLength(0)

    await user.type(input, '/')
    await user.click(within(dialog).getByRole('button', { name: /\{NNNN\}/ }))
    expect(input).toHaveValue('Z-{MM}/{NNNN}')
    expect(within(dialog).getByText('Z-03/0001')).toBeInTheDocument()
    expect(within(dialog).getByText('Číslování začíná každý měsíc znovu od 1.')).toBeInTheDocument()

    await user.type(input, '{{')
    // chyba u pole (po opuštění) i místo náhledu
    expect(within(dialog).getAllByText('Neuzavřená závorka „{“').length).toBeGreaterThan(0)
    await user.type(input, '{Backspace}')

    await user.click(within(dialog).getByRole('checkbox', { name: 'Nastavit jako výchozí řadu' }))
    await user.click(within(dialog).getByRole('button', { name: 'Přidat řadu' }))
    await waitFor(() => expect(created).toEqual([{ document_type: 'proforma', format: 'Z-{MM}/{NNNN}', is_default: true }]))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Nová řada/ })).not.toBeInTheDocument())
  })

  it('duplicitní řada ukáže českou chybu serveru', async () => {
    mockFormats([])
    const { user } = await openPage()
    await user.click(within(screen.getByRole('region', { name: 'Faktury' })).getByRole('button', { name: /Přidat/ }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Přidat řadu' }))
    expect(await within(dialog).findByText('Tato řada už pro daný typ dokladu existuje.')).toBeInTheDocument()
  })

  it('člen týmu jen čte', async () => {
    setSession('member')
    mockFormats([fmt({}), fmt({ id: 2, format: 'B{NNN}', is_default: false })])
    await openPage()
    expect(screen.getByText('Číselné řady může měnit jen vlastník nebo administrátor účtu.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Přidat/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Upravit řadu/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Nastavit jako výchozí' })).not.toBeInTheDocument()
  })
})
