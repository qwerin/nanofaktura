import { describe, expect, it } from 'vitest'
import { isValidIco } from './validation'

describe('isValidIco', () => {
  it.each(['27074358', '25596641', '00006947', '6947'])('accepts %s', (ico) => {
    expect(isValidIco(ico)).toBe(true)
  })
  it.each(['', '12345678', '2707435', 'abcdefgh', '270743581'])('rejects %j', (ico) => {
    expect(isValidIco(ico)).toBe(false)
  })
})
