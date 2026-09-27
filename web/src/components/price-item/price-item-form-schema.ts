// Formulář položky ceníku: schéma, výchozí hodnoty a převod na API (čisté funkce, testované).

import { z } from 'zod'
import type { Account, CreatePriceItemInput, PriceItem, UpdatePriceItemInput } from '@/api/types'
import { formatMoneyInput, parseMoney } from '@/lib/money'
import { parseQuantity } from '@/lib/quantity'

const optionalQuantity = (msg: string) => z.string().refine((v) => v.trim() === '' || parseQuantity(v) !== null, msg)

export const priceItemFormSchema = z.object({
  name: z.string().trim().min(1, 'Zadejte název').max(200, 'Nejvýše 200 znaků'),
  sku: z.string().trim().max(64, 'Nejvýše 64 znaků'),
  unit_name: z.string().trim().max(20, 'Nejvýše 20 znaků'),
  unit_price: z.string().refine((v) => v.trim() === '' || parseMoney(v) !== null, 'Neplatná cena'),
  vat_rate_bps: z.string(),
  prices_include_vat: z.boolean(),
  currency: z.string().min(3),
  track_stock: z.boolean(),
  stock_quantity: optionalQuantity('Neplatné množství'),
  min_stock: optionalQuantity('Neplatné množství'),
  note: z.string(),
})

export type PriceItemFormValues = z.infer<typeof priceItemFormSchema>

export function newPriceItemDefaults(account: Pick<Account, 'default_currency' | 'default_vat_rate_bps'> | undefined): PriceItemFormValues {
  return {
    name: '',
    sku: '',
    unit_name: 'ks',
    unit_price: '',
    vat_rate_bps: String(account?.default_vat_rate_bps ?? 2100),
    prices_include_vat: false,
    currency: account?.default_currency || 'CZK',
    track_stock: false,
    stock_quantity: '',
    min_stock: '',
    note: '',
  }
}

export function priceItemToFormValues(p: PriceItem): PriceItemFormValues {
  return {
    name: p.name,
    sku: p.sku,
    unit_name: p.unit_name,
    unit_price: formatMoneyInput(p.unit_price),
    vat_rate_bps: String(p.vat_rate_bps),
    prices_include_vat: p.prices_include_vat,
    currency: p.currency,
    track_stock: p.track_stock,
    stock_quantity: '',
    min_stock: p.min_stock.replace('.', ','),
    note: p.note,
  }
}

function common(v: PriceItemFormValues) {
  return {
    name: v.name.trim(),
    sku: v.sku.trim(),
    unit_name: v.unit_name.trim(),
    unit_price: parseMoney(v.unit_price) ?? 0,
    vat_rate_bps: Number(v.vat_rate_bps),
    prices_include_vat: v.prices_include_vat,
    currency: v.currency,
    track_stock: v.track_stock,
    min_stock: v.track_stock ? (parseQuantity(v.min_stock) ?? '') : '',
    note: v.note,
  }
}

export function toCreatePriceItemInput(v: PriceItemFormValues): CreatePriceItemInput {
  const initial = v.track_stock ? parseQuantity(v.stock_quantity) : null
  return { ...common(v), ...(initial && initial !== '0' ? { stock_quantity: initial } : {}) }
}

export function toUpdatePriceItemInput(v: PriceItemFormValues): UpdatePriceItemInput {
  return common(v)
}
