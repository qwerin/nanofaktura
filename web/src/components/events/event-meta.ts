// Vzhled událostí: rodina (prefix názvu), ikona a barevný tón. Názvy viz GET /events/catalog.

import {
  ArchiveIcon,
  ArchiveRestoreIcon,
  BanIcon,
  BellRingIcon,
  CircleAlertIcon,
  CircleCheckIcon,
  CircleDotIcon,
  EyeIcon,
  FilePenIcon,
  FilePlusIcon,
  FileTextIcon,
  LandmarkIcon,
  LinkIcon,
  LockIcon,
  LockOpenIcon,
  MailIcon,
  MailWarningIcon,
  PackageIcon,
  ReceiptIcon,
  RepeatIcon,
  SendIcon,
  Trash2Icon,
  TriangleAlertIcon,
  Undo2Icon,
  UnlinkIcon,
  UserIcon,
  WalletIcon,
  WebhookIcon,
  type LucideIcon,
} from 'lucide-react'

export type EventTone = 'default' | 'info' | 'success' | 'warning' | 'destructive' | 'muted'

export interface EventVisual {
  icon: LucideIcon
  tone: EventTone
}

export type EventFamily =
  | 'invoice'
  | 'payment'
  | 'email'
  | 'public'
  | 'expense'
  | 'subject'
  | 'price_item'
  | 'recurring'
  | 'bank'
  | 'webhook'
  | 'other'

/** Rodina události podle prefixu: `invoice.paid` → `invoice`, `expense_payment.created` → `expense`, `stock.low` → `price_item`. */
export function eventFamily(name: string): EventFamily {
  const prefix = name.split('.', 1)[0] ?? ''
  switch (prefix) {
    case 'invoice':
    case 'payment':
    case 'email':
    case 'public':
    case 'expense':
    case 'subject':
    case 'price_item':
    case 'recurring':
    case 'bank':
    case 'webhook':
      return prefix
    case 'expense_payment':
      return 'expense'
    case 'stock':
      return 'price_item'
    default:
      return 'other'
  }
}

/** Filtry aktivity: rodina → popisek a hodnota filtru `name` (prefix) pro API. */
export const eventFamilyFilters: { family: Exclude<EventFamily, 'other'>; label: string; prefixes: string[] }[] = [
  { family: 'invoice', label: 'Faktury', prefixes: ['invoice'] },
  { family: 'payment', label: 'Platby', prefixes: ['payment'] },
  { family: 'email', label: 'E-maily', prefixes: ['email'] },
  { family: 'expense', label: 'Náklady', prefixes: ['expense', 'expense_payment'] },
  { family: 'subject', label: 'Kontakty', prefixes: ['subject'] },
  { family: 'price_item', label: 'Ceník a sklad', prefixes: ['price_item', 'stock'] },
  { family: 'recurring', label: 'Pravidelné', prefixes: ['recurring'] },
  { family: 'bank', label: 'Banka', prefixes: ['bank'] },
  { family: 'webhook', label: 'Webhooky', prefixes: ['webhook'] },
]

const familyIcons: Record<EventFamily, LucideIcon> = {
  invoice: FileTextIcon,
  payment: WalletIcon,
  email: MailIcon,
  public: EyeIcon,
  expense: ReceiptIcon,
  subject: UserIcon,
  price_item: PackageIcon,
  recurring: RepeatIcon,
  bank: LandmarkIcon,
  webhook: WebhookIcon,
  other: CircleDotIcon,
}

/** Konkrétní události s vlastní ikonou/tónem (přebíjí výchozí vzhled rodiny). */
const specific: Record<string, EventVisual> = {
  'invoice.created': { icon: FilePlusIcon, tone: 'default' },
  'invoice.updated': { icon: FilePenIcon, tone: 'muted' },
  'invoice.sent': { icon: SendIcon, tone: 'info' },
  'invoice.paid': { icon: CircleCheckIcon, tone: 'success' },
  'invoice.overdue': { icon: CircleAlertIcon, tone: 'destructive' },
  'invoice.cancelled': { icon: BanIcon, tone: 'muted' },
  'invoice.cancel_undone': { icon: Undo2Icon, tone: 'muted' },
  'invoice.uncollectible': { icon: TriangleAlertIcon, tone: 'warning' },
  'invoice.uncollectible_undone': { icon: Undo2Icon, tone: 'muted' },
  'invoice.locked': { icon: LockIcon, tone: 'muted' },
  'invoice.unlocked': { icon: LockOpenIcon, tone: 'muted' },
  'invoice.public_link_regenerated': { icon: LinkIcon, tone: 'muted' },
  'payment.created': { icon: WalletIcon, tone: 'success' },
  'email.sent': { icon: MailIcon, tone: 'info' },
  'email.failed': { icon: MailWarningIcon, tone: 'destructive' },
  'public.viewed': { icon: EyeIcon, tone: 'info' },
  'expense.created': { icon: ReceiptIcon, tone: 'default' },
  'expense.updated': { icon: FilePenIcon, tone: 'muted' },
  'expense.paid': { icon: CircleCheckIcon, tone: 'success' },
  'expense.locked': { icon: LockIcon, tone: 'muted' },
  'expense.unlocked': { icon: LockOpenIcon, tone: 'muted' },
  'expense_payment.created': { icon: WalletIcon, tone: 'success' },
  'stock.low': { icon: TriangleAlertIcon, tone: 'warning' },
  'recurring.generated': { icon: RepeatIcon, tone: 'success' },
  'recurring.failed': { icon: CircleAlertIcon, tone: 'destructive' },
  'bank.matched': { icon: LinkIcon, tone: 'success' },
  'bank.unmatched': { icon: UnlinkIcon, tone: 'muted' },
  'webhook.failed': { icon: BellRingIcon, tone: 'destructive' },
  'webhook.disabled': { icon: BanIcon, tone: 'destructive' },
  'account.exported': { icon: ArchiveIcon, tone: 'muted' },
  'account.imported': { icon: ArchiveRestoreIcon, tone: 'info' },
}

/** Ikona a tón pro událost. Smazání je vždy „destructive“, neznámé názvy dostanou ikonu rodiny. */
export function eventVisual(name: string): EventVisual {
  const hit = specific[name]
  if (hit) return hit
  if (name.endsWith('.deleted')) return { icon: Trash2Icon, tone: 'destructive' }
  return { icon: familyIcons[eventFamily(name)], tone: 'default' }
}

/** Třídy „bubliny“ s ikonou podle tónu (barvy jen z tokenů). */
export const toneClasses: Record<EventTone, string> = {
  default: 'bg-primary/10 text-primary',
  info: 'bg-info/12 text-info dark:bg-info/20',
  success: 'bg-success/12 text-success dark:bg-success/20',
  warning: 'bg-warning/15 text-warning-foreground dark:text-warning',
  destructive: 'bg-destructive/10 text-destructive dark:bg-destructive/20',
  muted: 'bg-muted text-muted-foreground',
}

/** Na smazaný záznam už odkazovat nejde. */
export function eventLinksToRecord(name: string): boolean {
  return !name.endsWith('.deleted')
}
