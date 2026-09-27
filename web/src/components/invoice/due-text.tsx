import type { InvoiceStatus } from '@/api/types'
import { cn } from '@/lib/utils'
import { dueInfo, dueToneClass } from './status'

/** „splatná za 3 dny“ / „5 dní po splatnosti“ v barvě podle naléhavosti. */
export function DueText({
  invoice,
  className,
  hidePaid,
}: {
  invoice: { status: InvoiceStatus; due_on: string; paid_on: string }
  className?: string
  hidePaid?: boolean
}) {
  const info = dueInfo(invoice)
  if (!info || (hidePaid && info.tone === 'success')) return null
  return <span className={cn(dueToneClass[info.tone], info.tone !== 'muted' && 'font-medium', className)}>{info.text}</span>
}
