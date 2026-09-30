import { CircleCheckIcon, FileTextIcon, MailWarningIcon, ReceiptIcon, Trash2Icon, WalletIcon } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { eventFamily, eventLinksToRecord, eventVisual } from './event-meta'

describe('eventFamily', () => {
  it('maps prefixes', () => {
    expect(eventFamily('invoice.paid')).toBe('invoice')
    expect(eventFamily('expense_payment.created')).toBe('expense')
    expect(eventFamily('stock.low')).toBe('price_item')
    expect(eventFamily('mystery')).toBe('other')
  })
})

describe('eventVisual', () => {
  it('uses specific icons and tones', () => {
    expect(eventVisual('invoice.paid')).toEqual({ icon: CircleCheckIcon, tone: 'success' })
    expect(eventVisual('email.failed')).toEqual({ icon: MailWarningIcon, tone: 'destructive' })
    expect(eventVisual('payment.created').icon).toBe(WalletIcon)
  })
  it('treats deletes as destructive and falls back to the family icon', () => {
    expect(eventVisual('subject.deleted')).toEqual({ icon: Trash2Icon, tone: 'destructive' })
    expect(eventVisual('invoice.something_new')).toEqual({ icon: FileTextIcon, tone: 'default' })
    expect(eventVisual('expense_payment.deleted').icon).toBe(Trash2Icon)
    expect(eventVisual('expense.created').icon).toBe(ReceiptIcon)
  })
  it('does not link deleted records', () => {
    expect(eventLinksToRecord('invoice.deleted')).toBe(false)
    expect(eventLinksToRecord('invoice.sent')).toBe(true)
  })
})
