import { describe, expect, it } from 'vitest'
import { groupSecret, recoveryCodesText } from './two-factor'

describe('groupSecret', () => {
  it('groups the base32 secret by 4 characters', () => {
    expect(groupSecret('JBSWY3DPEHPK3PXP')).toBe('JBSW Y3DP EHPK 3PXP')
    expect(groupSecret('ABCDEF')).toBe('ABCD EF')
    expect(groupSecret('AB CD')).toBe('ABCD')
    expect(groupSecret('')).toBe('')
  })
})

describe('recoveryCodesText', () => {
  it('lists every code on its own line', () => {
    const text = recoveryCodesText(['aaaaa-bbbbb', 'ccccc-ddddd'], 'a@example.cz', new Date(2026, 8, 30, 12, 0))
    const lines = text.split('\n')
    expect(lines[0]).toBe('NanoFaktura – záložní kódy pro a@example.cz')
    expect(lines).toContain('aaaaa-bbbbb')
    expect(lines).toContain('ccccc-ddddd')
  })
})
