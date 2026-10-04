import { cn } from '@/lib/utils'
import { invoiceLabel } from './status'

/** Číslo dokladu; koncept bez čísla jako tlumené „Koncept“. */
export function InvoiceNumber({ number, className }: { number: string; className?: string }) {
  return <span className={cn(!number && 'font-normal text-muted-foreground italic', className)}>{invoiceLabel({ number })}</span>
}
