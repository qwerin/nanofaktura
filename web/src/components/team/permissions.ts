// Pravidla správy týmu (SPEC §7.13, „Otevřené otázky“ → Role, pozvánky).
// Zdrojem pravdy je backend (403/409) — tady jen skrýváme/zakazujeme akce, které by selhaly,
// a překládáme chyby do češtiny.

import { isApiError } from '@/api/errors'
import type { MemberRole } from '@/api/types'

export const ROLES: readonly MemberRole[] = ['owner', 'admin', 'accountant', 'member']

export const roleDescriptions: Record<MemberRole, string> = {
  owner: 'Plný přístup včetně správy dalších vlastníků.',
  admin: 'Vše kromě správy vlastníků — doklady, nastavení firmy i tým.',
  accountant: 'Vidí všechny doklady a nastavení, ale nic nemění. Pro vaši účetní.',
  member: 'Vystavuje faktury, spravuje kontakty a náklady. Do nastavení nesmí.',
}

export function isManager(role: MemberRole | undefined): boolean {
  return role === 'owner' || role === 'admin'
}

/** Role, které smí `actor` přidělit (pozvánkou nebo změnou role). Ownera smí udělit jen owner. */
export function assignableRoles(actor: MemberRole | undefined): MemberRole[] {
  if (actor === 'owner') return [...ROLES]
  if (actor === 'admin') return ROLES.filter((r) => r !== 'owner')
  return []
}

export interface TeamContext {
  /** Role přihlášeného uživatele v účtu. */
  actor: MemberRole | undefined
  /** Počet vlastníků v účtu. */
  ownerCount: number
}

interface Target {
  role: MemberRole
  isSelf: boolean
}

/** Je `target` poslední vlastník? Toho nejde degradovat ani odebrat (ani sám odejít). */
export function isLastOwner(target: Pick<Target, 'role'>, ctx: TeamContext): boolean {
  return target.role === 'owner' && ctx.ownerCount <= 1
}

/** Smí `actor` změnit roli člena? (Samotné volby omezuje `assignableRoles`.) */
export function canChangeRole(target: Target, ctx: TeamContext): boolean {
  if (!isManager(ctx.actor)) return false
  if (target.role === 'owner' && ctx.actor !== 'owner') return false
  // Poslední vlastník by mohl jen „změnit“ na vlastníka — nemá smysl nabízet.
  if (isLastOwner(target, ctx)) return false
  return true
}

/** Důvod, proč změna role není možná (pro nápovědu v UI), jinak `null`. */
export function changeRoleBlockedReason(target: Target, ctx: TeamContext): string | null {
  if (!isManager(ctx.actor)) return 'Role může měnit jen vlastník nebo administrátor.'
  if (target.role === 'owner' && ctx.actor !== 'owner') return 'Vlastníky může spravovat jen jiný vlastník.'
  if (isLastOwner(target, ctx)) return 'Účet musí mít alespoň jednoho vlastníka.'
  return null
}

/** Smí `actor` odebrat člena z účtu (u sebe = opustit účet)? */
export function canRemove(target: Target, ctx: TeamContext): boolean {
  if (isLastOwner(target, ctx)) return false
  if (target.isSelf) return true
  if (!isManager(ctx.actor)) return false
  return !(target.role === 'owner' && ctx.actor !== 'owner')
}

/** Smí `actor` zrušit (nebo znovu poslat) pozvánku s rolí `role`? */
export function canManageInvitation(role: MemberRole, actor: MemberRole | undefined): boolean {
  return isManager(actor) && (role !== 'owner' || actor === 'owner')
}

/** Český text chyby z API týmu (detaily backendu jsou anglicky). */
export function teamErrorMessage(err: unknown): string {
  if (!isApiError(err)) return 'Nastala neočekávaná chyba.'
  const detail = (err.problem.detail ?? '').toLowerCase()
  switch (err.status) {
    case 403:
      if (detail.includes('only an owner')) return 'Vlastníky může spravovat jen jiný vlastník.'
      if (detail.includes('different e-mail')) return 'Pozvánka byla poslána na jinou e-mailovou adresu.'
      return 'K této akci nemáte oprávnění.'
    case 404:
      return 'Člen nebo pozvánka už neexistuje. Obnovte stránku.'
    case 409:
      if (detail.includes('at least one owner')) return 'Účet musí mít alespoň jednoho vlastníka.'
      if (detail.includes('already a member')) {
        return detail.includes('you are') ? 'Už jste členem tohoto účtu.' : 'Tento uživatel už je členem účtu.'
      }
      return err.message
    case 410:
      return 'Pozvánka vypršela nebo už byla použita.'
    case 422: {
      const emailError = err.problem.errors?.some((e) => e.location === 'body.email')
      return emailError ? 'Zadejte platný e-mail.' : 'Zkontrolujte zadané údaje.'
    }
    case 502:
      return 'Pozvánku se nepodařilo odeslat e-mailem. Zkontrolujte nastavení e-mailu na serveru.'
    default:
      return err.message
  }
}
