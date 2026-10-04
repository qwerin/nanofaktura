import { screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { OAuthDecision, OAuthGrant } from '@/api/types'
import { TS } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'

// Žádost tak, jak ji pošle discovery (state číselný — nesmí projít JSON převodem routeru).
const query =
  'client_id=cl_abc&redirect_uri=https%3A%2F%2Fclaude.ai%2Fapi%2Fmcp%2Fauth_callback&response_type=code' +
  '&state=12345&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256' +
  '&scope=mcp&resource=http%3A%2F%2Flocalhost%3A3000%2Fapi%2Fmcp'
const url = `/oauth/authorize?${query}`

const expectedParams = {
  client_id: 'cl_abc',
  redirect_uri: 'https://claude.ai/api/mcp/auth_callback',
  response_type: 'code',
  state: '12345',
  code_challenge: 'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM',
  code_challenge_method: 'S256',
  scope: 'mcp',
  resource: 'http://localhost:3000/api/mcp',
}

const assign = vi.fn()

beforeEach(() => {
  assign.mockReset()
  vi.stubGlobal('location', { ...window.location, assign })
})
afterEach(() => vi.unstubAllGlobals())

function mockConsent() {
  const seen: Record<string, string>[] = []
  server.use(
    http.get(`${API}/api/oauth/authorize`, ({ request }) => {
      seen.push(Object.fromEntries(new URL(request.url).searchParams))
      return HttpResponse.json({ client_name: 'Claude', redirect_host: 'claude.ai', resource: expectedParams.resource })
    }),
  )
  return seen
}

function mockDecision() {
  const bodies: OAuthDecision[] = []
  server.use(
    http.post(`${API}/api/oauth/authorize`, async ({ request }) => {
      const body = (await request.json()) as OAuthDecision
      bodies.push(body)
      const target = body.approve ? 'code=xyz' : 'error=access_denied'
      return HttpResponse.json({ redirect_to: `https://claude.ai/api/mcp/auth_callback?${target}&state=12345` })
    }),
  )
  return bodies
}

describe('připojení aplikace (OAuth souhlas)', () => {
  it('ukáže aplikaci, návratovou adresu a přihlášeného uživatele', async () => {
    const seen = mockConsent()
    await renderApp(url)

    expect(await screen.findByText('Claude')).toBeInTheDocument()
    expect(screen.getByText(/chce přístup k vašemu účtu/)).toBeInTheDocument()
    expect(screen.getByText(/Název uvádí sama aplikace/)).toBeInTheDocument()
    expect(screen.getByText('claude.ai')).toBeInTheDocument()
    expect(screen.getByText(/Jan Novák \(jan@example\.cz\)/)).toBeInTheDocument()
    expect(screen.getByText(/Nastavení → API tokeny → Připojené aplikace/)).toBeInTheDocument()
    expect(seen).toEqual([expectedParams])
  })

  it('Povolit přístup odešle souhlas a pošle prohlížeč zpět do aplikace', async () => {
    mockConsent()
    const bodies = mockDecision()
    const { user } = await renderApp(url)

    await user.click(await screen.findByRole('button', { name: 'Povolit přístup' }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://claude.ai/api/mcp/auth_callback?code=xyz&state=12345'))
    expect(bodies).toEqual([{ ...expectedParams, approve: true }])
    // během odchodu jsou tlačítka zamčená
    expect(screen.getByRole('button', { name: /Povolit přístup/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: /Zamítnout/ })).toBeDisabled()
  })

  it('Zamítnout vrátí aplikaci odmítnutí', async () => {
    mockConsent()
    const bodies = mockDecision()
    const { user } = await renderApp(url)

    await user.click(await screen.findByRole('button', { name: 'Zamítnout' }))
    await waitFor(() => expect(assign).toHaveBeenCalledWith(expect.stringContaining('error=access_denied')))
    expect(bodies[0]?.approve).toBe(false)
  })

  it('neplatná žádost ukáže chybu a žádná tlačítka, nepřesměruje', async () => {
    server.use(
      http.get(`${API}/api/oauth/authorize`, () =>
        problem(422, 'unknown redirect uri', { code: 'oauth_invalid_redirect' }),
      ),
    )
    await renderApp(url)

    expect(await screen.findByText('Aplikaci nelze připojit')).toBeInTheDocument()
    expect(screen.getByText(/adresu, kterou při registraci neuvedla/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Povolit přístup' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Zamítnout' })).not.toBeInTheDocument()
    expect(assign).not.toHaveBeenCalled()
  })

  it('nepřihlášeného pošle na přihlášení a po něm zpět s celou žádostí', async () => {
    setSession(null)
    const { router } = await renderApp(url)
    await waitFor(() => expect(router.state.location.pathname).toBe('/login'))
    const redirect = (router.state.location.search as { redirect?: string }).redirect ?? ''
    const back = new URL(redirect, 'http://x')
    expect(back.pathname).toBe('/oauth/authorize')
    expect(Object.fromEntries(back.searchParams)).toEqual(expectedParams)
  })
})

describe('připojené aplikace v nastavení', () => {
  const grant: OAuthGrant = { id: 7, client_name: 'Claude', created_at: TS }

  it('vypíše aplikace a odpojí je', async () => {
    let grants = [grant]
    const deleted: string[] = []
    server.use(
      http.get(`${API}/api/auth/tokens`, () => HttpResponse.json({ items: [], total: 0, page: 1, per_page: 50 })),
      http.get(`${API}/api/auth/oauth-grants`, () =>
        HttpResponse.json({ items: grants, total: grants.length, page: 1, per_page: 50 }),
      ),
      http.delete(`${API}/api/auth/oauth-grants/:id`, ({ params }) => {
        deleted.push(String(params.id))
        grants = []
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = await renderApp('/a/firma/settings/tokens')

    const section = await screen.findByRole('region', { name: 'Připojené aplikace' })
    expect(await within(section).findByRole('cell', { name: 'Claude' })).toBeInTheDocument()
    expect(within(section).getByRole('cell', { name: 'Nikdy' })).toBeInTheDocument()

    await user.click(within(section).getByRole('button', { name: 'Odpojit' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Odpojit aplikaci?')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Odpojit' }))

    expect(await screen.findByText('Aplikace odpojena')).toBeInTheDocument()
    expect(deleted).toEqual(['7'])
    expect(await within(section).findByText('Žádné připojené aplikace')).toBeInTheDocument()
  })
})
