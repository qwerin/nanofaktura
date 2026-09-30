// Navigace přihlášené části: nastavení (skupiny, všechny sekce), odznak Novinek, paleta příkazů, role, mobil.
import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { SearchHit } from '@/api/types'
import { settingsGroups, settingsNav } from '@/components/layout/nav'
import { newsEntries } from '@/hooks/use-news'
import { renderApp } from '@/test/render'
import { API, server, setSession } from '@/test/server'
import { setMobile } from '@/test/viewport'

const NEWS_KEY = 'nf.news.lastSeen'

describe('nastavení — navigace', () => {
  it('desktop: všechny sekce ve skupinách a každá je dosažitelná', async () => {
    const { user, router } = await renderApp('/a/firma/settings')
    // /settings na desktopu rovnou otevře první sekci
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/settings/company'))
    const nav = await screen.findByRole('navigation', { name: 'Sekce nastavení' })
    for (const g of settingsGroups) expect(within(nav).getByText(g.label)).toBeInTheDocument()
    const links = within(nav).getAllByRole('link')
    expect(links.map((l) => l.textContent)).toEqual(settingsNav.map((i) => i.label))

    for (const item of settingsNav) {
      await user.click(within(nav).getByRole('link', { name: item.label }))
      await waitFor(() => expect(router.state.location.pathname).toBe(item.to.replace('$slug', 'firma')))
      expect(within(nav).getByRole('link', { name: item.label })).toHaveAttribute('data-status', 'active')
    }
  })

  it('mobil: /settings je seznam sekcí po skupinách', async () => {
    setMobile()
    const { user, router } = await renderApp('/a/firma/settings')
    for (const g of settingsGroups) expect(await screen.findByRole('heading', { level: 2, name: g.label })).toBeInTheDocument()
    for (const item of settingsNav) expect(screen.getByRole('link', { name: new RegExp(item.label) })).toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Sekce nastavení' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('link', { name: /Číselné řady/ }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/settings/number-formats'))
  })
})

describe('novinky — odznak', () => {
  it('desktop: „Nové“ u Novinek zmizí po otevření stránky', async () => {
    const { user } = await renderApp('/a/firma/todos')
    const sidebar = await screen.findByRole('link', { name: 'Novinky' })
    const item = sidebar.closest('li')!
    expect(within(item).getByText('Nové')).toBeInTheDocument()
    await user.click(sidebar)
    await screen.findByRole('heading', { level: 1, name: 'Novinky' })
    await waitFor(() => expect(within(item).queryByText('Nové')).not.toBeInTheDocument())
    expect(localStorage.getItem(NEWS_KEY)).toBe(newsEntries[0]!.date)
  })

  it('přečtené novinky odznak nemají', async () => {
    localStorage.setItem(NEWS_KEY, newsEntries[0]!.date)
    await renderApp('/a/firma/todos')
    const item = (await screen.findByRole('link', { name: 'Novinky' })).closest('li')!
    expect(within(item).queryByText('Nové')).not.toBeInTheDocument()
  })

  it('mobil: tečka na „Více“ a pilulka v menu', async () => {
    setMobile()
    const { user } = await renderApp('/a/firma/todos')
    const more = await screen.findByRole('button', { name: /Více/ })
    expect(within(more).getByLabelText('Nové novinky')).toBeInTheDocument()
    await user.click(more)
    const drawer = await screen.findByRole('dialog', { name: 'Více' })
    const row = within(drawer).getByRole('button', { name: /Novinky/ })
    expect(within(row).getByText('Nové')).toBeInTheDocument()
    // mobilní menu obsahuje i všechny sekce nastavení
    for (const item of settingsNav) expect(within(drawer).getByRole('button', { name: item.label })).toBeInTheDocument()
  })
})

describe('uživatelské menu', () => {
  it('správa instance jen pro správce instance', async () => {
    setSession('owner', {}, { instance_admin: true })
    const { user, router } = await renderApp('/a/firma/todos')
    await user.click(await screen.findByRole('button', { name: /Jan Novák/ }))
    await user.click(await screen.findByRole('menuitem', { name: 'Správa instance' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/admin'))
  })

  it('běžný vlastník ji nevidí; přepnutí na tmavý motiv', async () => {
    const { user } = await renderApp('/a/firma/todos')
    await user.click(await screen.findByRole('button', { name: /Jan Novák/ }))
    const menu = await screen.findByRole('menu')
    expect(within(menu).getByRole('menuitem', { name: 'Můj profil' })).toBeInTheDocument()
    expect(within(menu).queryByRole('menuitem', { name: 'Správa instance' })).not.toBeInTheDocument()

    await user.click(within(menu).getByRole('menuitemradio', { name: /Tmavý/ }))
    await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
    expect(localStorage.getItem('nf-theme')).toBe('dark')
  })
})

const hit = (o: Partial<SearchHit>): SearchHit => ({ id: 1, type: 'invoice', title: '', subtitle: '', url_hint: '', ...o })

function mockSearch() {
  const queries: string[] = []
  server.use(
    http.get(`${API}/api/accounts/:slug/search`, ({ request }) => {
      const q = new URL(request.url).searchParams.get('q') ?? ''
      queries.push(q)
      return HttpResponse.json({
        invoices: q.includes('acme') ? [hit({ id: 42, title: '2026-0001', subtitle: 'ACME a.s.', status: 'overdue', url_hint: '/a/firma/invoices/42' })] : [],
        expenses: [],
        subjects: q.includes('acme') ? [hit({ id: 7, type: 'subject', title: 'ACME a.s.', subtitle: 'IČO 87654321', url_hint: '/a/firma/subjects/7' })] : [],
        price_items: [],
      })
    }),
  )
  return queries
}

describe('paleta příkazů', () => {
  it('Ctrl+K otevře hledání; výsledek otevře detail', async () => {
    const queries = mockSearch()
    const { user, router } = await renderApp('/a/firma/todos')
    await screen.findByRole('link', { name: 'Novinky' })
    await user.keyboard('{Control>}k{/Control}')
    const dialog = await screen.findByRole('dialog', { name: 'Hledat' })
    // bez dotazu: akce + hlavní sekce
    expect(within(dialog).getByRole('option', { name: /Nová faktura/ })).toBeInTheDocument()
    expect(within(dialog).getByRole('option', { name: /Kontakty/ })).toBeInTheDocument()

    await user.keyboard('acme')
    const inv = await within(dialog).findByRole('option', { name: /2026-0001/ })
    expect(within(inv).getByText('Po splatnosti')).toBeInTheDocument()
    expect(queries.at(-1)).toBe('acme')
    await user.click(inv)
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/42'))
    expect(screen.queryByRole('dialog', { name: 'Hledat' })).not.toBeInTheDocument()
  })

  it('u kontaktu nabídne „Nová faktura pro …“ a hledá i v nastavení', async () => {
    mockSearch()
    const { user, router } = await renderApp('/a/firma/todos')
    await user.click(await screen.findByRole('button', { name: /Hledat/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Hledat' })
    await user.keyboard('acme')
    const subject = await within(dialog).findByRole('option', { name: /ACME a\.s\..*IČO/ })
    await user.hover(subject)
    const forSubject = await within(dialog).findByRole('option', { name: /Nová faktura pro ACME/ })
    await user.click(forSubject)
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/new'))
    expect(router.state.location.search).toMatchObject({ subject_id: 7 })

    await user.keyboard('{Control>}k{/Control}')
    const again = await screen.findByRole('dialog', { name: 'Hledat' })
    await user.keyboard('řady')
    await user.click(await within(again).findByRole('option', { name: /Nastavení › Číselné řady/ }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/settings/number-formats'))
  })

  it('nic nenalezeno', async () => {
    mockSearch()
    const { user } = await renderApp('/a/firma/todos')
    await screen.findByRole('link', { name: 'Novinky' })
    await user.keyboard('{Control>}k{/Control}')
    const dialog = await screen.findByRole('dialog', { name: 'Hledat' })
    await user.keyboard('xyzzy')
    expect(await within(dialog).findByText('Nic nenalezeno pro „xyzzy“.')).toBeInTheDocument()
  })

  it('účetní nemá akce pro vytváření dokladů', async () => {
    setSession('accountant')
    mockSearch()
    const { user, router } = await renderApp('/a/firma/todos')
    await screen.findByRole('link', { name: 'Novinky' })
    await user.keyboard('{Control>}k{/Control}')
    const dialog = await screen.findByRole('dialog', { name: 'Hledat' })
    expect(within(dialog).queryByRole('option', { name: /Nová faktura/ })).not.toBeInTheDocument()
    expect(within(dialog).queryByRole('option', { name: /Nový náklad/ })).not.toBeInTheDocument()
    expect(within(dialog).getByRole('option', { name: /Faktury/ })).toBeInTheDocument()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Hledat' })).not.toBeInTheDocument())
    // zkratka N účetní nikam nepřesune
    await user.keyboard('n')
    expect(router.state.location.pathname).toBe('/a/firma/todos')
  })

  it('zkratka N otevře novou fakturu (mimo pole)', async () => {
    const { user, router } = await renderApp('/a/firma/todos')
    await screen.findByRole('link', { name: 'Novinky' })
    await user.keyboard('n')
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices/new'))
  })

  it('mobil: celoobrazovková paleta s „Zrušit“, bez klávesových zkratek', async () => {
    setMobile()
    mockSearch()
    const { user } = await renderApp('/a/firma/todos')
    await user.click(await screen.findByRole('button', { name: 'Hledat' }))
    const dialog = await screen.findByRole('dialog', { name: 'Hledat' })
    expect(within(dialog).queryByRole('option', { name: /Klávesové zkratky/ })).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Zrušit' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Hledat' })).not.toBeInTheDocument())
  })
})
