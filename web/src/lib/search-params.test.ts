import { describe, expect, it } from 'vitest'
import { parseSearch, stringifySearch } from './search-params'

describe('search params', () => {
  it('parametry OAuth nechá jako řetězce, ostatní převede jako dřív', () => {
    const search = parseSearch('?state=12345&scope=true&code_challenge=1e3&page=2&q=abc')
    expect(search).toEqual({ state: '12345', scope: 'true', code_challenge: '1e3', page: 2, q: 'abc' })
  })

  it('zápis je symetrický se čtením a zachová pořadí', () => {
    const str = stringifySearch({ client_id: 'c1', state: '007', page: 2, redirect: '/oauth/authorize?state=1' })
    expect(str).toBe('?client_id=c1&state=007&page=2&redirect=%2Foauth%2Fauthorize%3Fstate%3D1')
    expect(parseSearch(str)).toEqual({ client_id: 'c1', state: '007', page: 2, redirect: '/oauth/authorize?state=1' })
    expect(stringifySearch({ q: '123' })).toBe('?q=%22123%22')
    expect(stringifySearch({})).toBe('')
  })
})
