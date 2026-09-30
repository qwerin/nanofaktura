// Skrývání akcí podle role (SPEC §7.13): účetní jen čte, člen nemá nastavení.
import { screen, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { AccountMembership } from '@/api/types'
import { emptyList, makeSubject } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, server, setSession } from '@/test/server'
import { setMobile } from '@/test/viewport'

type Role = AccountMembership['role']

function mockLists() {
  const empty = () => HttpResponse.json({ ...emptyList(), sums: [] })
  server.use(
    http.get(`${API}/api/accounts/:slug/invoices`, empty),
    http.get(`${API}/api/accounts/:slug/expenses`, empty),
    http.get(`${API}/api/accounts/:slug/subjects`, empty),
    http.get(`${API}/api/accounts/:slug/subjects/:id`, () => HttpResponse.json(makeSubject())),
  )
}

/** Tlačítko nebo odkaz s daným názvem (akce hlavičky jsou odkazy vykreslené jako tlačítka). */
function queryAction(name: string | RegExp, container: HTMLElement = document.body) {
  const q = within(container)
  return q.queryAllByRole('link', { name }).concat(q.queryAllByRole('button', { name }))
}

const cases: { role: Role; edit: boolean }[] = [
  { role: 'owner', edit: true },
  { role: 'admin', edit: true },
  { role: 'member', edit: true },
  { role: 'accountant', edit: false },
]

describe.each(cases)('role $role', ({ role, edit }) => {
  it(`faktury: „Nová faktura“ ${edit ? 'je' : 'není'} v seznamu, prázdném stavu ani navigaci`, async () => {
    setSession(role)
    mockLists()
    await renderApp('/a/firma/invoices')
    await screen.findByText('Zatím žádné faktury')
    const sidebar = screen.getAllByRole('link', { name: 'Faktury' })[0]!.closest('[data-sidebar="content"]') as HTMLElement
    expect(queryAction('Nová faktura', sidebar).length > 0).toBe(edit)
    const main = screen.getByRole('main')
    expect(queryAction('Nová faktura', main).length > 0).toBe(edit)
    expect(queryAction('Vystavit fakturu', main).length > 0).toBe(edit)
  })

  it(`náklady a kontakty: vytváření ${edit ? 'povoleno' : 'skryto'}`, async () => {
    setSession(role)
    mockLists()
    await renderApp('/a/firma/expenses')
    await screen.findByRole('heading', { level: 1, name: 'Náklady' })
    expect(queryAction(/Nový náklad/).length > 0).toBe(edit)

    await renderApp('/a/firma/subjects')
    await screen.findAllByRole('heading', { level: 1, name: 'Kontakty' })
    expect(queryAction(/Nový kontakt/).length > 0).toBe(edit)
  })

  it(`detail kontaktu: úpravy ${edit ? 'povoleny' : 'skryty'}`, async () => {
    setSession(role)
    mockLists()
    await renderApp('/a/firma/subjects/7')
    await screen.findByRole('heading', { level: 1, name: 'ACME a.s.' })
    expect(queryAction('Upravit').length > 0).toBe(edit)
    expect(queryAction('Nová faktura').length > 0).toBe(edit)
  })
})

describe('mobil: tlačítko + v tab baru', () => {
  it.each(cases)('$role', async ({ role, edit }) => {
    setMobile()
    setSession(role)
    mockLists()
    await renderApp('/a/firma/invoices')
    const tabbar = await screen.findByRole('navigation', { name: 'Hlavní navigace' })
    expect(within(tabbar).queryByRole('link', { name: 'Nová faktura' }) !== null).toBe(edit)
    // ostatní záložky vidí každý
    expect(within(tabbar).getByRole('link', { name: 'Faktury' })).toBeInTheDocument()
  })
})
