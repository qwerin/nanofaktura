import { describe, expect, it } from 'vitest'
import { addressLines, displayWeb, formatAddress, formatZip, telHref, webHref } from './contact'

describe('formatZip', () => {
  it('groups Czech and Slovak ZIP codes', () => {
    expect(formatZip('11000')).toBe('110 00')
    expect(formatZip('81101', 'SK')).toBe('811 01')
    expect(formatZip('110 00')).toBe('110 00')
    expect(formatZip('10115', 'DE')).toBe('10115')
  })
})

describe('addressLines / formatAddress', () => {
  it('formats a Czech address without country', () => {
    const a = { street: 'Hlavní 1', city: 'Praha', zip: '11000', country: 'CZ' }
    expect(addressLines(a)).toEqual(['Hlavní 1', '110 00 Praha'])
    expect(formatAddress(a)).toBe('Hlavní 1, 110 00 Praha')
  })
  it('adds the country name abroad and skips empty parts', () => {
    expect(addressLines({ city: 'Bratislava', country: 'SK' })).toEqual(['Bratislava', 'Slovensko'])
    expect(formatAddress({})).toBe('')
  })
})

describe('links', () => {
  it('builds hrefs', () => {
    expect(webHref('example.cz')).toBe('https://example.cz')
    expect(webHref('http://example.cz')).toBe('http://example.cz')
    expect(displayWeb('https://www.example.cz/')).toBe('www.example.cz')
    expect(telHref('+420 777 123 456')).toBe('tel:+420777123456')
  })
})
