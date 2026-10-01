import { describe, expect, it } from 'vitest'
import { paymentAmountError, precheckFinalInvoice, remainderOnFinalInvoice } from './payment-logic'

describe('paymentAmountError', () => {
  it('requires a non-zero amount', () => {
    expect(paymentAmountError(null, 1000)).toBe('Zadejte nenulovou částku')
    expect(paymentAmountError(0, 1000)).toBe('Zadejte nenulovou částku')
  })
  it('requires the sign of the remaining amount', () => {
    expect(paymentAmountError(500, 1000)).toBeNull()
    expect(paymentAmountError(-500, 1000)).toMatch(/kladná/)
    // dobropis: vrácení peněz je záporná platba
    expect(paymentAmountError(-500, -1000)).toBeNull()
    expect(paymentAmountError(500, -1000)).toMatch(/minus/)
  })
  it('allows any sign when nothing remains (overpayment / correction)', () => {
    expect(paymentAmountError(-100, 0)).toBeNull()
    expect(paymentAmountError(100, 0)).toBeNull()
  })
})

describe('precheckFinalInvoice', () => {
  it('is checked only for the full remaining amount', () => {
    expect(precheckFinalInvoice(121_000, 121_000)).toBe(true)
    expect(precheckFinalInvoice(40_000, 121_000)).toBe(false)
    expect(precheckFinalInvoice(null, 121_000)).toBe(false)
    expect(precheckFinalInvoice(0, 0)).toBe(false)
  })
})

describe('remainderOnFinalInvoice', () => {
  it('computes the rest left on the final invoice', () => {
    expect(remainderOnFinalInvoice(40_000, 121_000)).toBe(81_000)
    expect(remainderOnFinalInvoice(121_000, 121_000)).toBe(0)
    expect(remainderOnFinalInvoice(150_000, 121_000)).toBe(0)
    expect(remainderOnFinalInvoice(null, 121_000)).toBe(0)
  })
})
