// Historie e-mailů dokladu (GET /invoices/{id}/emails): druh, příjemci, čas, chyba, automatika.

import { useQuery } from '@tanstack/react-query'
import { BellRingIcon, BotIcon, ChevronDownIcon, HeartHandshakeIcon, MailIcon, PaperclipIcon, TriangleAlertIcon } from 'lucide-react'
import { useState } from 'react'
import { emailQueries } from '@/api/queries/emails'
import type { EmailKind, EmailLog } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime } from '@/lib/date'
import { cn } from '@/lib/utils'
import { emailKindLabels } from './placeholders'

const kindIcons: Record<EmailKind, typeof MailIcon> = {
  invoice: MailIcon,
  reminder: BellRingIcon,
  paid_thanks: HeartHandshakeIcon,
}

export function EmailHistory({ slug, invoiceId }: { slug: string; invoiceId: number }) {
  const list = useQuery(emailQueries.invoiceEmails(slug, invoiceId))
  if (list.isPending) {
    return (
      <div className="flex flex-col gap-2" aria-busy="true">
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
      </div>
    )
  }
  if (list.isError) return <p className="text-sm text-destructive">Historii se nepodařilo načíst.</p>
  const items = list.data.items
  if (items.length === 0) return <p className="text-sm text-muted-foreground">Zatím nic neodešlo.</p>
  return (
    <ul className="-my-1 divide-y">
      {items.map((e) => (
        <EmailRow key={e.id} e={e} />
      ))}
    </ul>
  )
}

function EmailRow({ e }: { e: EmailLog }) {
  const [open, setOpen] = useState(false)
  const Icon = e.error ? TriangleAlertIcon : kindIcons[e.kind]
  const recipients = [...e.to, ...e.cc]
  return (
    <li className="py-2">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-start gap-3 rounded-md text-left focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        <span
          className={cn(
            'mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-full',
            e.error ? 'bg-destructive/10 text-destructive' : 'bg-primary/10 text-primary',
          )}
        >
          <Icon className="size-4" />
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex flex-wrap items-center gap-1.5">
            <span className="text-sm font-medium">{emailKindLabels[e.kind]}</span>
            {e.automatic && (
              <Badge variant="secondary" className="gap-1">
                <BotIcon data-icon="inline-start" />
                {e.reminder_step > 0 ? `automaticky · ${e.reminder_step} dní po splatnosti` : 'automaticky'}
              </Badge>
            )}
            {e.error && <Badge variant="destructive">Neodesláno</Badge>}
          </span>
          <span className="truncate text-xs text-muted-foreground">
            {recipients.length ? recipients.join(', ') : 'bez příjemce'} · {formatDateTime(e.sent_at ?? e.created_at)}
          </span>
          {e.error && <span className="mt-0.5 text-xs break-words text-destructive">{e.error}</span>}
        </span>
        <ChevronDownIcon className={cn('mt-2 size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="mt-2 ml-11 flex flex-col gap-1.5 rounded-lg bg-muted/50 p-3 text-sm">
          <p className="font-medium break-words">{e.subject}</p>
          {e.cc.length > 0 && <p className="text-xs text-muted-foreground">Kopie: {e.cc.join(', ')}</p>}
          <p className="text-sm break-words whitespace-pre-line text-muted-foreground">{e.body}</p>
          {e.attachments.length > 0 && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <PaperclipIcon className="size-3.5" />
              {e.attachments.join(', ')}
            </p>
          )}
        </div>
      )}
    </li>
  )
}
