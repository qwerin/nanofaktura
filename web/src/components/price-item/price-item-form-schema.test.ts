import { describe, expect, it } from 'vitest'
import type { PriceItem } from '@/api/types'
import {
  newPriceItemDefaults,
  priceItemFormSchema,
  priceItemToFormValues,
  toCreatePriceItemInput,
  toUpdatePriceItemInput,
  type PriceItemFormValues,
} from './price-item-form-schema'

const base = (patch: Partial<PriceItemFormValues> = {}): PriceItemFormValues => ({
  ...newPriceItemDefaults({ default_currency: 'CZK', default_vat_rate_bps: 2100 }),
  name: 'Kabel USB-C',
  unit_price: '199,00',
  ...patch,
})

describe('price item form', () => {
  it('validates quantities and price', () => {
    expect(priceItemFormSchema.safeParse(base()).success).toBe(true)
    const r = priceItemFormSchema.safeParse(base({ name: '', unit_price: 'x', min_stock: '1,2345' }))
    expect(r.error?.issues.map((i) => i.path.join('.')).sort()).toEqual(['min_stock', 'name', 'unit_price'])
  })

  it('create sends initial stock only with track_stock', () => {
    expect(toCreatePriceItemInput(base({ stock_quantity: '10' }))).not.toHaveProperty('stock_quantity')
    const input = toCreatePriceItemInput(base({ track_stock: true, stock_quantity: '10,5', min_stock: '2' }))
    expect(input).toMatchObject({ unit_price: 19900, vat_rate_bps: 2100, track_stock: true, stock_quantity: '10.5', min_stock: '2' })
    expect(toCreatePriceItemInput(base({ track_stock: true, stock_quantity: '0' }))).not.toHaveProperty('stock_quantity')
  })

  it('update clears min_stock when stock is not tracked', () => {
    expect(toUpdatePriceItemInput(base({ track_stock: false, min_stock: '5' })).min_stock).toBe('')
    expect(toUpdatePriceItemInput(base({ unit_price: '' })).unit_price).toBe(0)
  })

  it('maps a saved item to form values', () => {
    const p = { name: 'A', sku: 'X1', unit_name: 'kg', unit_price: 1050, vat_rate_bps: 1200, prices_include_vat: true, currency: 'EUR', track_stock: true, min_stock: '2.5', note: '' } as PriceItem
    expect(priceItemToFormValues(p)).toMatchObject({ unit_price: '10,50', vat_rate_bps: '1200', min_stock: '2,5', stock_quantity: '' })
  })
})
