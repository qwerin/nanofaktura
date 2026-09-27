import { describe, expect, it } from 'vitest'
import {
  bankName,
  checkCzechAccount,
  czechAccountToIban,
  formatIban,
  isValidIban,
  isValidSwift,
  parseCzechAccount,
} from './bank'

describe('checkCzechAccount', () => {
  it('parses prefix, number and bank', () => {
    expect(parseCzechAccount('19-2000145399/0800')).toEqual({ prefix: '19', number: '2000145399', bank: '0800' })
    expect(parseCzechAccount(' 2000145399 / 0800 ')).toEqual({ prefix: '', number: '2000145399', bank: '0800' })
  })
  it('reports format errors', () => {
    expect(checkCzechAccount('')).toEqual({ error: 'format' })
    expect(checkCzechAccount('2000145399')).toEqual({ error: 'format' })
    expect(checkCzechAccount('2000145399/80')).toEqual({ error: 'format' })
    expect(checkCzechAccount('1234567-2000145399/0800')).toEqual({ error: 'format' })
  })
  it('reports mod-11 failures per part', () => {
    expect(checkCzechAccount('2000145398/0800')).toEqual({ error: 'number' })
    expect(checkCzechAccount('18-2000145399/0800')).toEqual({ error: 'prefix' })
  })
})

describe('czechAccountToIban', () => {
  it.each([
    ['19-2000145399/0800', 'CZ6508000000192000145399'],
    ['2000145399/0800', 'CZ7908000000002000145399'],
  ])('%s → %s', (input, iban) => {
    const acc = parseCzechAccount(input)
    expect(acc).not.toBeNull()
    const got = czechAccountToIban(acc!)
    expect(got).toBe(iban)
    expect(isValidIban(got)).toBe(true)
  })
})

describe('isValidIban', () => {
  it.each(['CZ65 0800 0000 1920 0014 5399', 'DE89370400440532013000', 'sk3112000000198742637541'])('accepts %s', (v) =>
    expect(isValidIban(v)).toBe(true),
  )
  it.each(['', 'CZ6508000000192000145390', 'DE8937040044053201300', 'XX'])('rejects %j', (v) =>
    expect(isValidIban(v)).toBe(false),
  )
})

describe('formatIban / isValidSwift / bankName', () => {
  it('groups IBAN by four', () => {
    expect(formatIban('cz6508000000192000145399')).toBe('CZ65 0800 0000 1920 0014 5399')
  })
  it('validates SWIFT', () => {
    expect(isValidSwift('GIBACZPX')).toBe(true)
    expect(isValidSwift('fiobczppxxx')).toBe(true)
    expect(isValidSwift('GIBA')).toBe(false)
  })
  it('knows common banks', () => {
    expect(bankName('2010')).toBe('Fio banka')
    expect(bankName('9999')).toBeUndefined()
  })
})
