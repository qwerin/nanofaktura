import {
  ArchiveIcon,
  BuildingIcon,
  ChartColumnIcon,
  FileTextIcon,
  HashIcon,
  KeyRoundIcon,
  LandmarkIcon,
  LayoutDashboardIcon,
  ListTodoIcon,
  MailIcon,
  PackageIcon,
  RepeatIcon,
  ReceiptIcon,
  PaletteIcon,
  WebhookIcon,
  UserCogIcon,
  UserIcon,
  UsersIcon,
} from 'lucide-react'

// Jediný zdroj pravdy pro navigaci (sidebar, tab bar, „Více“, nastavení).

/** Položky, které na mobilu nejsou v tab baru — zobrazují se v menu „Více“ (na desktopu v sidebaru). */
export const moreNav = [
  { label: 'Úkoly', to: '/a/$slug/todos', icon: ListTodoIcon },
  { label: 'Náklady', to: '/a/$slug/expenses', icon: ReceiptIcon },
  { label: 'Ceník', to: '/a/$slug/price-items', icon: PackageIcon },
  { label: 'Pravidelné faktury', to: '/a/$slug/recurring', icon: RepeatIcon },
  { label: 'Banka', to: '/a/$slug/bank', icon: LandmarkIcon },
  { label: 'Přehledy', to: '/a/$slug/reports', icon: ChartColumnIcon },
] as const

export const mainNav = [
  { label: 'Přehled', to: '/a/$slug/dashboard', icon: LayoutDashboardIcon },
  { label: 'Faktury', to: '/a/$slug/invoices', icon: FileTextIcon },
  { label: 'Kontakty', to: '/a/$slug/subjects', icon: UsersIcon },
  ...moreNav,
] as const

export const settingsNav = [
  { label: 'Firma', to: '/a/$slug/settings/company', icon: BuildingIcon, description: 'Fakturační údaje a výchozí hodnoty' },
  { label: 'Bankovní účty', to: '/a/$slug/settings/bank-accounts', icon: LandmarkIcon, description: 'Účty pro platby a QR kód' },
  { label: 'Číselné řady', to: '/a/$slug/settings/number-formats', icon: HashIcon, description: 'Formát čísel dokladů' },
  { label: 'Vzhled dokladů', to: '/a/$slug/settings/appearance', icon: PaletteIcon, description: 'Logo, podpis a náhled PDF' },
  { label: 'Tým', to: '/a/$slug/settings/members', icon: UserCogIcon, description: 'Uživatelé účtu, role a pozvánky' },
  { label: 'Můj profil', to: '/a/$slug/settings/profile', icon: UserIcon, description: 'Jméno a heslo' },
  { label: 'API tokeny', to: '/a/$slug/settings/tokens', icon: KeyRoundIcon, description: 'Přístup pro integrace' },
  { label: 'E-maily a upomínky', to: '/a/$slug/settings/emails', icon: MailIcon, description: 'Texty e-mailů, podpis, automatické upomínky' },
  { label: 'Webhooky', to: '/a/$slug/settings/webhooks', icon: WebhookIcon, description: 'Oznámení o událostech do jiných aplikací' },
  { label: 'Záloha a přenos', to: '/a/$slug/settings/backup', icon: ArchiveIcon, description: 'Stažení zálohy a obnova účtu' },
] as const

/** Iniciály pro avatar účtu/uživatele: „Jan Novák“ → „JN“. */
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  const first = parts[0]?.[0] ?? '?'
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : (parts[0]?.[1] ?? '')
  return (first + second).toUpperCase()
}
