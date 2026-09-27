// Role v účtu (SPEC §7.13). Zdrojem pravdy je backend (403), tady jen skrýváme nedostupné akce.

import type { AccountMembership } from '@/api/types'

export type Role = AccountMembership['role']

export const roleLabels: Record<Role, string> = {
  owner: 'Vlastník',
  admin: 'Administrátor',
  accountant: 'Účetní',
  member: 'Člen',
}

export function roleLabel(role: Role | undefined): string {
  return role ? roleLabels[role] : ''
}

/** Nastavení firmy, bankovní účty, číselné řady, uživatelé. */
export function canManageSettingsRole(role: Role | undefined): boolean {
  return role === 'owner' || role === 'admin'
}

/** Doklady, kontakty, náklady, přílohy. Účetní má jen čtení. */
export function canEditRole(role: Role | undefined): boolean {
  return role === 'owner' || role === 'admin' || role === 'member'
}
