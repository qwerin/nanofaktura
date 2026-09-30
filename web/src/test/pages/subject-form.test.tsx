import { screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { AresSubject, CreateSubjectInput } from '@/api/types'
import { makeSubject } from '@/test/fixtures'
import { renderApp } from '@/test/render'
import { API, problem, server } from '@/test/server'

const VALID_ICO = '27074358'

const ares: AresSubject = {
  registration_no: VALID_ICO,
  name: 'Firma z ARES s.r.o.',
  vat_no: 'CZ27074358',
  street: 'Na Příkopě 1',
  city: 'Praha',
  zip: '11000',
  country: 'CZ',
}

function mockSubjects({ existing = [] as ReturnType<typeof makeSubject>[] } = {}) {
  const calls = { ares: [] as string[], created: [] as CreateSubjectInput[] }
  server.use(
    http.get(`${API}/api/ares/:ico`, ({ params }) => {
      calls.ares.push(String(params.ico))
      if (params.ico === '00000019') return problem(404, 'not found')
      if (params.ico === '00000027') return problem(502, 'ares down')
      return HttpResponse.json(ares)
    }),
    http.get(`${API}/api/accounts/:slug/subjects`, () =>
      HttpResponse.json({ items: existing, total: existing.length, page: 1, per_page: 5 }),
    ),
    http.post(`${API}/api/accounts/:slug/subjects`, async ({ request }) => {
      const body = (await request.json()) as CreateSubjectInput
      calls.created.push(body)
      if (body.email === 'spatny@') {
        return problem(422, 'validation failed', { errors: [{ location: 'body.email', message: 'neplatný e-mail (server)' }] })
      }
      return HttpResponse.json(makeSubject({ id: 99, ...body }), { status: 201 })
    }),
    http.get(`${API}/api/accounts/:slug/subjects/:id`, () => HttpResponse.json(makeSubject({ id: 99 }))),
  )
  return calls
}

async function openNew() {
  const app = await renderApp('/a/firma/subjects/new')
  await screen.findByRole('heading', { level: 1, name: 'Nový kontakt' })
  return app
}

describe('formulář kontaktu', () => {
  it('platné IČO se samo dohledá v ARES a doplní údaje; uloží se jen vyplněná pole', async () => {
    const calls = mockSubjects()
    const { user, router } = await openNew()
    await user.type(screen.getByLabelText('IČO'), VALID_ICO)

    await waitFor(() => expect(screen.getByLabelText('Název firmy / jméno')).toHaveValue('Firma z ARES s.r.o.'))
    expect(screen.getByLabelText('DIČ')).toHaveValue('CZ27074358')
    expect(screen.getByLabelText('Ulice a číslo')).toHaveValue('Na Příkopě 1')
    expect(screen.getByLabelText('Město')).toHaveValue('Praha')
    expect(screen.getByLabelText('PSČ')).toHaveValue('11000')
    expect(screen.getByText(`Načteno z ARES: ${ares.name}`)).toBeInTheDocument()
    expect(calls.ares).toEqual([VALID_ICO])

    // stejné IČO znovu tlačítkem → výsledek z cache, žádný další dotaz
    await user.click(screen.getByRole('button', { name: 'Načíst z ARES' }))
    expect(calls.ares).toHaveLength(1)

    await user.click(screen.getByRole('button', { name: /^(Uložit|Vytvořit)/ }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/a/firma/subjects/99'))
    expect(calls.created[0]).toMatchObject({ registration_no: VALID_ICO, name: ares.name, vat_no: ares.vat_no, city: 'Praha' })
    // prázdná pole se neposílají
    expect(calls.created[0]).not.toHaveProperty('email')
  })

  it('neplatné IČO neodešle dotaz a ukáže chybu', async () => {
    const calls = mockSubjects()
    const { user } = await openNew()
    await user.type(screen.getByLabelText('IČO'), '12345678')
    await user.click(screen.getByRole('button', { name: 'Načíst z ARES' }))
    expect(await screen.findByText('Zadejte platné IČO (8 číslic).')).toBeInTheDocument()
    expect(calls.ares).toHaveLength(0)
  })

  it('nenalezené IČO a nedostupný ARES mají srozumitelné hlášky', async () => {
    mockSubjects()
    const { user } = await openNew()
    const ico = screen.getByLabelText('IČO')
    await user.type(ico, '19{Enter}')
    expect(await screen.findByText('Subjekt s tímto IČO v ARES není.')).toBeInTheDocument()
    await user.clear(ico)
    await user.type(ico, '27{Enter}')
    expect(await screen.findByText('ARES je momentálně nedostupný, vyplňte údaje ručně.')).toBeInTheDocument()
    // formulář zůstane použitelný pro ruční vyplnění
    expect(screen.getByLabelText('Název firmy / jméno')).toBeEnabled()
  })

  it('upozorní na existující kontakt se stejným IČO', async () => {
    mockSubjects({ existing: [makeSubject({ id: 5, name: 'Už existuje', registration_no: VALID_ICO })] })
    const { user } = await openNew()
    await user.type(screen.getByLabelText('IČO'), VALID_ICO)
    expect(await screen.findByRole('link', { name: 'Už existuje' })).toHaveAttribute('href', '/a/firma/subjects/5')
  })

  it('422 ze serveru se ukáže u pole', async () => {
    const calls = mockSubjects()
    const { user } = await openNew()
    await user.type(screen.getByLabelText('Název firmy / jméno'), 'Ručně')
    await user.type(screen.getByLabelText('E-mail'), 'spatny@')
    await user.click(screen.getByRole('button', { name: /^(Uložit|Vytvořit)/ }))
    // klientská validace e-mailu může zachytit dřív — obě varianty musí pole označit jako chybné
    await waitFor(() => expect(screen.getByLabelText('E-mail')).toHaveAttribute('aria-invalid', 'true'))
    if (calls.created.length) expect(screen.getByText('neplatný e-mail (server)')).toBeInTheDocument()
  })
})
