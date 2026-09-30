import { describe, expect, it } from 'vitest'
import { recordHref } from './record-links'

describe('recordHref', () => {
  it('builds detail paths', () => {
    expect(recordHref('acme', 'invoice', 12)).toBe('/a/acme/invoices/12')
    expect(recordHref('acme', 'price_item', 3)).toBe('/a/acme/price-items/3')
    expect(recordHref('acme', 'bank_transaction', 9)).toBe('/a/acme/bank')
    expect(recordHref('acme', 'webhook', 1)).toBe('/a/acme/settings/webhooks')
  })
  it('returns null for unknown or incomplete input', () => {
    expect(recordHref('acme', undefined, 1)).toBeNull()
    expect(recordHref('acme', 'invoice', undefined)).toBeNull()
    expect(recordHref('acme', 'alien', 1)).toBeNull()
  })
})
