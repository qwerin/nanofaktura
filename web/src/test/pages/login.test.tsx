import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { LoginInput } from '@/api/types'
import { makeMe } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server, setSession } from '@/test/server'

const challenge = { token: 'chal-1', methods: ['totp', 'recovery'] as ('totp' | 'recovery')[] }

describe('přihlášení', () => {
  it('bez 2FA přihlásí a přesměruje na redirect', async () => {
    setSession(null)
    let body: LoginInput | undefined
    server.use(
      http.post(`${API}/api/auth/login`, async ({ request }) => {
        body = (await request.json()) as LoginInput
        return HttpResponse.json({ me: makeMe() })
      }),
    )
    const { user, router } = await renderApp('/login?redirect=%2Fa%2Ffirma%2Finvoices')

    await user.type(await screen.findByLabelText('E-mail'), '  Jan@Example.CZ ')
    await user.type(screen.getByLabelText('Heslo'), 'tajne-heslo')
    await user.click(screen.getByRole('button', { name: 'Přihlásit se' }))

    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/invoices'))
    // e-mail se normalizuje (trim + malá písmena)
    expect(body).toEqual({ email: 'jan@example.cz', password: 'tajne-heslo' })
  })

  it('validuje pole a špatné heslo ukáže česky', async () => {
    setSession(null)
    server.use(http.post(`${API}/api/auth/login`, () => problem(401, 'invalid credentials')))
    const { user } = await renderApp('/login')

    await user.click(await screen.findByRole('button', { name: 'Přihlásit se' }))
    expect(await screen.findByText('Zadejte platný e-mail')).toBeInTheDocument()
    expect(screen.getByText('Zadejte heslo')).toBeInTheDocument()

    await user.type(screen.getByLabelText('E-mail'), 'jan@example.cz')
    await user.type(screen.getByLabelText('Heslo'), 'spatne')
    await user.click(screen.getByRole('button', { name: 'Přihlásit se' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Nesprávný e-mail nebo heslo.')
  })

  it('na čisté instanci nabídne vytvoření prvního účtu', async () => {
    setSession(null)
    server.use(http.get(`${API}/api/auth/status`, () => HttpResponse.json({ has_users: false, signup_allowed: true })))
    await renderApp('/login')
    expect(await screen.findByRole('link', { name: 'Vytvořte první účet' })).toHaveAttribute('href', '/register')
  })

  it('přihlášeného uživatele rovnou přesměruje', async () => {
    const { router } = await renderApp('/login?redirect=%2Fa%2Ffirma%2Fsubjects')
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/subjects'))
  })

  describe('druhý krok (2FA)', () => {
    async function toSecondStep() {
      setSession(null)
      server.use(http.post(`${API}/api/auth/login`, () => HttpResponse.json({ two_factor: challenge })))
      const app = await renderApp('/login')
      await app.user.type(await screen.findByLabelText('E-mail'), 'jan@example.cz')
      await app.user.type(screen.getByLabelText('Heslo'), 'heslo123')
      await app.user.click(screen.getByRole('button', { name: 'Přihlásit se' }))
      await screen.findByText('Ověření přihlášení')
      return app
    }

    it('kód z aplikace dokončí přihlášení', async () => {
      let sent: unknown
      server.use(
        http.post(`${API}/api/auth/login/2fa`, async ({ request }) => {
          sent = await request.json()
          return HttpResponse.json(makeMe())
        }),
      )
      const { user, router } = await toSecondStep()
      await user.click(screen.getByRole('button', { name: 'Ověřit' }))
      expect(await screen.findByText('Zadejte 6místný kód.')).toBeInTheDocument()

      // 6. číslice odešle kód sama (bez klepnutí na Ověřit)
      await user.type(screen.getByLabelText('Kód z aplikace'), '123456')
      await waitFor(() => expect(sent).toEqual({ token: 'chal-1', code: '123456' }))
      await waitFor(() => expect(router.state.location.pathname).not.toBe('/login'))
    })

    it('špatný kód ukáže chybu, záložní kód funguje', async () => {
      const codes: string[] = []
      server.use(
        http.post(`${API}/api/auth/login/2fa`, async ({ request }) => {
          const { code } = (await request.json()) as { code: string }
          codes.push(code)
          if (code === '000000') return problem(401, 'invalid code', { code: 'invalid_code' })
          return HttpResponse.json(makeMe())
        }),
      )
      const { user, router } = await toSecondStep()
      await user.keyboard('000000')
      expect(await screen.findByText('Kód není správný. Zadejte aktuální kód z aplikace.')).toBeInTheDocument()

      await user.click(screen.getByRole('button', { name: 'Použít záložní kód' }))
      await user.click(screen.getByRole('button', { name: 'Ověřit záložní kód' }))
      expect(await screen.findByText('Zadejte záložní kód.')).toBeInTheDocument()
      await user.type(screen.getByLabelText('Záložní kód'), ' abcde-12345 ')
      await user.click(screen.getByRole('button', { name: 'Ověřit záložní kód' }))
      await waitFor(() => expect(router.state.location.pathname).not.toBe('/login'))
      expect(codes).toEqual(['000000', 'abcde-12345'])
    })

    it('vypršelá výzva vrátí na heslo s hláškou', async () => {
      server.use(
        http.post(`${API}/api/auth/login/2fa`, () => problem(401, 'expired', { code: 'two_factor_expired' })),
      )
      const { user } = await toSecondStep()
      await user.keyboard('123456')
      expect(await screen.findByRole('button', { name: 'Přihlásit se' })).toBeInTheDocument()
      expect(screen.getByRole('alert')).not.toBeEmptyDOMElement()
      expect(screen.getByLabelText('Heslo')).toHaveValue('')
    })

    it('Zpět vrátí na formulář s heslem', async () => {
      const { user } = await toSecondStep()
      await user.click(screen.getByRole('button', { name: 'Zpět' }))
      expect(await screen.findByRole('button', { name: 'Přihlásit se' })).toBeInTheDocument()
      expect(screen.getByLabelText('E-mail')).toHaveValue('jan@example.cz')
    })
  })
})
