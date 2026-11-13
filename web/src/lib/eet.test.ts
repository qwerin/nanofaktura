import { describe, expect, it } from 'vitest'
import {
  bytesToBase64,
  canRetrySale,
  certificateEicMismatch,
  certificateWarning,
  certificateWarningText,
  defaultContactPayment,
  deleteSendsCancellation,
  eetStatusLabels,
  eetStatusTones,
  eicError,
  failedSummary,
  fileToBase64,
  isCancellation,
  isPastDeadline,
  pendingSummary,
  registerIdError,
  salesCountText,
  splitInvoiceSales,
  suggestEic,
  unitIdError,
} from './eet'

describe('stavy tržeb', () => {
  it('má popisek a tón pro každý stav', () => {
    expect(eetStatusLabels).toEqual({
      confirmed: 'Zaevidováno',
      pending: 'Čeká na odeslání',
      failed: 'Odmítnuto',
      cancelled: 'Zrušeno',
    })
    expect(eetStatusTones).toEqual({ confirmed: 'success', pending: 'warning', failed: 'destructive', cancelled: 'muted' })
  })

  it('znovu odeslat jde jen čekající a odmítnutou', () => {
    expect(canRetrySale({ status: 'pending' })).toBe(true)
    expect(canRetrySale({ status: 'failed' })).toBe(true)
    expect(canRetrySale({ status: 'confirmed' })).toBe(false)
    expect(canRetrySale({ status: 'cancelled' })).toBe(false)
  })

  it('storno pozná podle odkazu na rušenou tržbu nebo záporné částky', () => {
    expect(isCancellation({ cancels_id: 5, amount: -100 })).toBe(true)
    expect(isCancellation({ amount: -100 })).toBe(true)
    expect(isCancellation({ amount: 100 })).toBe(false)
  })

  it('smazání platby posílá storno jen u zaevidované nebo čekající tržby', () => {
    expect(deleteSendsCancellation({ status: 'confirmed' })).toBe(true)
    expect(deleteSendsCancellation({ status: 'pending' })).toBe(true)
    expect(deleteSendsCancellation({ status: 'failed' })).toBe(false)
    expect(deleteSendsCancellation({ status: 'cancelled' })).toBe(false)
    expect(deleteSendsCancellation(undefined)).toBe(false)
  })

  it('lhůta 48 hodin se hlídá jen u nezaevidovaných tržeb', () => {
    const now = new Date('2027-01-10T12:00:00Z')
    expect(isPastDeadline({ status: 'pending', deadline_at: '2027-01-10T11:59:00Z' }, now)).toBe(true)
    expect(isPastDeadline({ status: 'failed', deadline_at: '2027-01-11T00:00:00Z' }, now)).toBe(false)
    expect(isPastDeadline({ status: 'confirmed', deadline_at: '2027-01-01T00:00:00Z' }, now)).toBe(false)
  })
})

describe('defaultContactPayment', () => {
  it('předvolí hotovost a kartu, ostatní ne', () => {
    expect(defaultContactPayment('cash')).toBe(true)
    expect(defaultContactPayment('card')).toBe(true)
    for (const m of ['bank', 'cod', 'paypal', 'custom', '']) expect(defaultContactPayment(m)).toBe(false)
  })
})

describe('splitInvoiceSales', () => {
  const sale = (id: number, payment_id?: number, extra: { cancels_id?: number; amount?: number } = {}) => ({
    id,
    payment_id,
    amount: 1000,
    ...extra,
  })

  it('přiřadí tržby k platbám, storna a tržby smazaných plateb nechá zvlášť', () => {
    const a = sale(1, 10)
    const deleted = sale(2, undefined)
    const storno = sale(3, undefined, { cancels_id: 2, amount: -1000 })
    const foreign = sale(4, 99)
    const { byPayment, other } = splitInvoiceSales([{ id: 10 }, { id: 11 }], [a, deleted, storno, foreign])
    expect(byPayment.get(10)).toBe(a)
    expect(byPayment.has(11)).toBe(false)
    expect(other).toEqual([deleted, storno, foreign])
  })

  it('u platby s více záznamy ukáže nejnovější a starší nezahodí', () => {
    const older = sale(1, 10)
    const newer = sale(5, 10)
    const { byPayment, other } = splitInvoiceSales([{ id: 10 }], [newer, older])
    expect(byPayment.get(10)).toBe(newer)
    expect(other).toEqual([older])
    const swapped = splitInvoiceSales([{ id: 10 }], [older, newer])
    expect(swapped.byPayment.get(10)).toBe(newer)
    expect(swapped.other).toEqual([older])
  })
})

describe('údaje pro evidenci', () => {
  it('EIČ: CZ a 8–10 číslic', () => {
    expect(eicError('CZ12345678')).toBeNull()
    expect(eicError('CZ1234567890')).toBeNull()
    expect(eicError('CZ1234567')).not.toBeNull()
    expect(eicError('CZ12345678901')).not.toBeNull()
    expect(eicError('12345678')).not.toBeNull()
    expect(eicError('')).not.toBeNull()
  })

  it('evidenční jednotka: 1–9 číslic bez úvodní nuly', () => {
    expect(unitIdError('1')).toBeNull()
    expect(unitIdError('123456789')).toBeNull()
    expect(unitIdError('0123')).not.toBeNull()
    expect(unitIdError('1234567890')).not.toBeNull()
    expect(unitIdError('12a')).not.toBeNull()
    expect(unitIdError('')).not.toBeNull()
  })

  it('označení pokladny: nejvýše 20 povolených znaků', () => {
    expect(registerIdError('1')).toBeNull()
    expect(registerIdError('Pokl-1_A/2 #3.,:;')).toBeNull()
    expect(registerIdError('')).not.toBeNull()
    expect(registerIdError('x'.repeat(21))).not.toBeNull()
    expect(registerIdError('pokladní')).not.toBeNull()
  })

  it('navrhne EIČ z českého DIČ, jen když EIČ chybí', () => {
    expect(suggestEic('CZ12345678', '')).toBe('CZ12345678')
    expect(suggestEic(' cz 1234567890 ', '  ')).toBe('CZ1234567890')
    expect(suggestEic('CZ12345678', 'CZ87654321')).toBeNull()
    expect(suggestEic('SK2020202020', '')).toBeNull()
    expect(suggestEic('', '')).toBeNull()
    expect(suggestEic(undefined, '')).toBeNull()
  })
})

describe('certifikát', () => {
  const now = new Date('2027-03-01T12:00:00Z')

  it('varuje po vypršení a 30 dní předem', () => {
    expect(certificateWarning('2027-03-01T11:00:00Z', now)).toEqual({ level: 'expired' })
    expect(certificateWarning('2027-03-02T13:00:00Z', now)).toEqual({ level: 'expiring', days: 1 })
    expect(certificateWarning('2027-03-30T12:00:00Z', now)).toEqual({ level: 'expiring', days: 29 })
    expect(certificateWarning('2027-03-31T12:00:00Z', now)).toBeNull()
    expect(certificateWarning('2028-01-01T00:00:00Z', now)).toBeNull()
    expect(certificateWarning(undefined, now)).toBeNull()
    expect(certificateWarning('nesmysl', now)).toBeNull()
  })

  it('text varování skloňuje dny', () => {
    expect(certificateWarningText({ level: 'expired' })).toContain('vypršela')
    expect(certificateWarningText({ level: 'expiring', days: 0 })).toContain('vyprší dnes')
    expect(certificateWarningText({ level: 'expiring', days: 1 })).toContain('vyprší zítra')
    expect(certificateWarningText({ level: 'expiring', days: 3 })).toContain('za 3 dny')
    expect(certificateWarningText({ level: 'expiring', days: 12 })).toContain('za 12 dní')
  })

  it('pozná certifikát vystavený na jiné EIČ', () => {
    expect(certificateEicMismatch('CZ12345678', 'CZ87654321')).toBe(true)
    expect(certificateEicMismatch('CZ12345678', 'cz12345678 ')).toBe(false)
    expect(certificateEicMismatch(undefined, 'CZ12345678')).toBe(false)
    expect(certificateEicMismatch('CZ12345678', '')).toBe(false)
  })

  it('převede bajty a soubor na base64', async () => {
    expect(bytesToBase64(new Uint8Array([]))).toBe('')
    expect(bytesToBase64(new Uint8Array([0x30, 0x82, 0xff, 0x00]))).toBe('MIL/AA==')
    const big = new Uint8Array(100_000).fill(0x61)
    expect(atob(bytesToBase64(big))).toHaveLength(100_000)
    await expect(fileToBase64(new Blob([new Uint8Array([0x30, 0x82, 0xff, 0x00])]))).resolves.toBe('MIL/AA==')
  })
})

describe('texty přehledu', () => {
  it('skloňuje počty tržeb', () => {
    expect(salesCountText(1)).toBe('1 tržba')
    expect(salesCountText(3)).toBe('3 tržby')
    expect(salesCountText(5)).toBe('5 tržeb')
    expect(pendingSummary(1)).toMatch(/^1 tržba čeká na odeslání/)
    expect(pendingSummary(2)).toMatch(/^2 tržby čekají na odeslání/)
    expect(pendingSummary(7)).toMatch(/^7 tržeb čeká na odeslání/)
    expect(failedSummary(1)).toMatch(/^1 tržbu Finanční správa odmítla .* odešlete ji znovu/)
    expect(failedSummary(4)).toMatch(/^4 tržby Finanční správa odmítla .* odešlete je znovu/)
  })
})
