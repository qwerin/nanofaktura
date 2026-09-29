import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api/errors'
import {
  amountQuery,
  amountToneClass,
  bankSearchSchema,
  baseFilters,
  classifySyncError,
  countBankFilters,
  directionOf,
  formatSignedAmount,
  formatSyncedAgo,
  importErrorText,
  isAccountPublished,
  isCzechVatNo,
  isValidFioToken,
  matchErrorText,
  parseRetryAfter,
  rankCandidates,
  searchToFilters,
  sortReasons,
  suggestionConfidence,
  suggestionReason,
  symbolsText,
  syncErrorText,
  tabOf,
  transactionTitle,
} from './format'

const nbsp = (s: string) => s.replace(/[\u00a0\u202f]/g, ' ')

describe('formatSignedAmount', () => {
  it('prefixes incoming with plus and outgoing with a real minus', () => {
    expect(nbsp(formatSignedAmount(1210000, 'CZK'))).toBe('+12 100,00 Kč')
    expect(nbsp(formatSignedAmount(-4660, 'CZK'))).toBe('−46,60 Kč')
    expect(nbsp(formatSignedAmount(0, 'CZK'))).toBe('0,00 Kč')
  })
  it('works for foreign currency', () => {
    expect(nbsp(formatSignedAmount(-150050, 'EUR'))).toBe('−1 500,50 €')
  })
  it('direction and tone follow the sign', () => {
    expect(directionOf(1)).toBe('in')
    expect(directionOf(-1)).toBe('out')
    expect(directionOf(0)).toBe('in')
    expect(amountToneClass(5)).toBe('text-success')
    expect(amountToneClass(-5)).toBe('text-foreground')
    expect(amountToneClass(0)).toBe('text-muted-foreground')
  })
})

describe('suggestion reasons', () => {
  it('maps backend reasons to short chips with tone', () => {
    expect(suggestionReason('VS sedí')).toEqual({ label: 'VS sedí', tone: 'strong' })
    expect(suggestionReason('Částka sedí')).toEqual({ label: 'Částka sedí', tone: 'strong' })
    expect(suggestionReason('Částečná úhrada')).toEqual({ label: 'Částečná úhrada', tone: 'partial' })
    expect(suggestionReason('Částka odpovídá celkové částce dokladu').label).toBe('Celková částka sedí')
    expect(suggestionReason('Jméno protistrany sedí').label).toBe('Jméno sedí')
    expect(suggestionReason('Účet protistrany sedí').label).toBe('Účet sedí')
  })
  it('keeps unknown reasons as they are', () => {
    expect(suggestionReason('Něco nového')).toEqual({ label: 'Něco nového', tone: 'weak' })
  })
  it('sorts strong reasons first, stable otherwise', () => {
    expect(sortReasons(['Jméno protistrany sedí', 'Částečná úhrada', 'VS sedí']).map((r) => r.label)).toEqual([
      'VS sedí',
      'Částečná úhrada',
      'Jméno sedí',
    ])
  })
  it('confidence from score', () => {
    expect(suggestionConfidence(90)).toBe('high')
    expect(suggestionConfidence(50)).toBe('medium')
    expect(suggestionConfidence(20)).toBe('low')
  })
})

describe('filters in URL', () => {
  it('parses and sanitizes search params', () => {
    expect(bankSearchSchema.parse({ tab: 'matched', account: '3', direction: 'in', since: '2026-09-01', query: ' acme ' })).toEqual({
      tab: 'matched',
      account: 3,
      direction: 'in',
      since: '2026-09-01',
      query: 'acme',
    })
    expect(bankSearchSchema.parse({ tab: 'nonsense', account: 'x', direction: 'up', since: '1.9.2026', query: '' })).toEqual({})
  })
  it('default tab is unmatched and maps to state', () => {
    expect(tabOf({})).toBe('unmatched')
    expect(searchToFilters({})).toEqual({ state: 'unmatched' })
    expect(searchToFilters({ tab: 'suggested', account: 2 })).toEqual({ state: 'suggested', bank_account_id: 2 })
  })
  it('tab "all" sends no state', () => {
    expect(searchToFilters({ tab: 'all', direction: 'out', until: '2026-09-30', query: 'lidl' })).toEqual({
      direction: 'out',
      until: '2026-09-30',
      query: 'lidl',
    })
  })
  it('base filters ignore the tab; counter ignores tab and query', () => {
    expect(baseFilters({ tab: 'matched', account: 1 })).toEqual({ bank_account_id: 1 })
    expect(countBankFilters({ tab: 'all', query: 'x' })).toBe(0)
    expect(countBankFilters({ account: 1, direction: 'in', since: '2026-01-01', until: '2026-02-01' })).toBe(3)
  })
})

describe('transaction texts', () => {
  const t = { counterparty_name: '', message: '', counterparty_account: '', variable_symbol: '', constant_symbol: '', specific_symbol: '' }
  it('title falls back name → message → account', () => {
    expect(transactionTitle({ ...t, counterparty_name: 'ACME', message: 'x' })).toBe('ACME')
    expect(transactionTitle({ ...t, message: 'Platba kartou' })).toBe('Platba kartou')
    expect(transactionTitle({ ...t, counterparty_account: '123/0100' })).toBe('123/0100')
    expect(transactionTitle(t)).toBe('Bez popisu')
  })
  it('symbols only when filled', () => {
    expect(symbolsText({ ...t, variable_symbol: '2026001', constant_symbol: '0308' })).toBe('VS 2026001 · KS 0308')
    expect(symbolsText(t)).toBe('')
  })
})

describe('manual matching', () => {
  it('recognizes amount queries', () => {
    expect(amountQuery('12 100')).toBe(1210000)
    expect(amountQuery('12100,50 Kč')).toBe(1210050)
    expect(amountQuery('2026001')).toBeNull() // číslo dokladu / VS
    expect(amountQuery('FV-2026')).toBeNull()
    expect(amountQuery('')).toBeNull()
    expect(amountQuery('0')).toBeNull()
  })
  it('ranks exact remaining first, then total, unpaid, paid; drops other currency and cancelled', () => {
    const c = (id: number, total: number, remaining: number, extra: Partial<{ currency: string; payable: boolean }> = {}) => ({
      id,
      total,
      remaining,
      currency: 'CZK',
      payable: true,
      ...extra,
    })
    const ranked = rankCandidates(
      [c(1, 500, 0), c(2, 1000, 400), c(3, 1000, 1000), c(4, 400, 400, { currency: 'EUR' }), c(5, 400, 400, { payable: false }), c(6, 400, 200), c(7, 900, 400)],
      -400,
      'CZK',
    )
    expect(ranked.map((x) => x.id)).toEqual([2, 7, 6, 3, 1])
  })
})

describe('sync errors', () => {
  it('parses Retry-After', () => {
    expect(parseRetryAfter('30')).toBe(30)
    expect(parseRetryAfter(null)).toBe(30)
    expect(parseRetryAfter('abc')).toBe(30)
    expect(parseRetryAfter('Thu, 01 Jan 2026 00:00:20 GMT', Date.parse('2026-01-01T00:00:00Z'))).toBe(20)
  })
  it('classifies statuses', () => {
    expect(classifySyncError(new ApiError(429, {}), 12)).toEqual({ kind: 'rate_limited', retryAfter: 12 })
    expect(classifySyncError(new ApiError(409, { detail: 'automatic sync is not configured' }))).toEqual({ kind: 'not_configured' })
    expect(classifySyncError(new ApiError(422, { detail: 'Fio rejected the API token' }))).toEqual({ kind: 'bad_token' })
    expect(classifySyncError(new ApiError(422, { detail: 'too many transactions in the period' }))).toEqual({ kind: 'too_many' })
    expect(classifySyncError(new ApiError(502, {})).kind).toBe('other')
  })
  it('countdown text', () => {
    expect(syncErrorText({ kind: 'rate_limited', retryAfter: 30 })).toContain('zkuste za 30 s')
  })
})

describe('import / match error texts', () => {
  it('translates import problems', () => {
    const other = new ApiError(422, {
      detail: 'validation failed',
      errors: [{ location: 'body.file', message: 'the statement belongs to another bank account (CZ7920100000002000000000)' }],
    })
    expect(importErrorText(other)).toBe('Výpis patří k jinému bankovnímu účtu (CZ7920100000002000000000). Vyberte správný účet.')
    expect(importErrorText(new ApiError(422, { errors: [{ location: 'body.file', message: 'cannot read the statement: x' }] }))).toContain(
      'nepodařilo přečíst',
    )
    expect(importErrorText(new ApiError(413, { detail: 'file is larger than 10 MB' }))).toBe('Soubor je větší než 10 MB.')
  })
  it('translates match problems', () => {
    expect(matchErrorText(new ApiError(409, { detail: 'the transaction is already matched; unmatch it first' }))).toContain('už je spárovaná')
    expect(matchErrorText(new ApiError(422, { errors: [{ message: 'the invoice is in EUR, the transaction in CZK' }] }))).toBe(
      'Doklad je v jiné měně než platba.',
    )
  })
})

describe('misc helpers', () => {
  it('synced ago', () => {
    const now = Date.parse('2026-09-27T12:00:00Z')
    const abs = () => 'ABS'
    expect(formatSyncedAgo('2026-09-27T11:59:40Z', now, abs)).toBe('před chvílí')
    expect(formatSyncedAgo('2026-09-27T11:55:00Z', now, abs)).toBe('před 5 min')
    expect(formatSyncedAgo('2026-09-27T09:00:00Z', now, abs)).toBe('před 3 h')
    expect(formatSyncedAgo('2026-09-20T09:00:00Z', now, abs)).toBe('ABS')
  })
  it('fio token', () => {
    expect(isValidFioToken('a'.repeat(64))).toBe(true)
    expect(isValidFioToken('short')).toBe(false)
    expect(isValidFioToken('abc-def-ghi-jkl-mno')).toBe(false)
  })
  it('czech VAT numbers', () => {
    expect(isCzechVatNo('CZ27082440')).toBe(true)
    expect(isCzechVatNo('27082440')).toBe(true)
    expect(isCzechVatNo('SK2020317068')).toBe(false)
    expect(isCzechVatNo('')).toBe(false)
  })
  it('published account check', () => {
    const published = [{ number: '000019-2000145399/0800', iban: 'CZ6508000000192000145399' }]
    expect(isAccountPublished({ bank_account: '19-2000145399/0800' }, published)).toBe(true)
    expect(isAccountPublished({ iban: 'CZ65 0800 0000 1920 0014 5399' }, published)).toBe(true)
    expect(isAccountPublished({ bank_account: '123/0100' }, published)).toBe(false)
    expect(isAccountPublished({}, published)).toBeNull()
  })
})
