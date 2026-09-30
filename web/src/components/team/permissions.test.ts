import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api/errors'
import {
  assignableRoles,
  canChangeRole,
  canManageInvitation,
  canRemove,
  changeRoleBlockedReason,
  isLastOwner,
  teamErrorMessage,
} from './permissions'

const owner = { actor: 'owner' as const, ownerCount: 2 }
const admin = { actor: 'admin' as const, ownerCount: 1 }
const member = { actor: 'member' as const, ownerCount: 1 }

describe('assignableRoles', () => {
  it('owner can grant any role, admin all but owner, others none', () => {
    expect(assignableRoles('owner')).toEqual(['owner', 'admin', 'accountant', 'member'])
    expect(assignableRoles('admin')).toEqual(['admin', 'accountant', 'member'])
    expect(assignableRoles('accountant')).toEqual([])
    expect(assignableRoles('member')).toEqual([])
    expect(assignableRoles(undefined)).toEqual([])
  })
})

describe('canChangeRole', () => {
  it('only managers change roles', () => {
    expect(canChangeRole({ role: 'member', isSelf: false }, member)).toBe(false)
    expect(canChangeRole({ role: 'member', isSelf: false }, admin)).toBe(true)
  })
  it('admins cannot manage owners', () => {
    expect(canChangeRole({ role: 'owner', isSelf: false }, { actor: 'admin', ownerCount: 2 })).toBe(false)
    expect(changeRoleBlockedReason({ role: 'owner', isSelf: false }, admin)).toMatch(/jiný vlastník/)
    expect(canChangeRole({ role: 'owner', isSelf: false }, owner)).toBe(true)
  })
  it('protects the last owner', () => {
    const single = { actor: 'owner' as const, ownerCount: 1 }
    expect(isLastOwner({ role: 'owner' }, single)).toBe(true)
    expect(canChangeRole({ role: 'owner', isSelf: true }, single)).toBe(false)
    expect(changeRoleBlockedReason({ role: 'owner', isSelf: true }, single)).toMatch(/alespoň jednoho vlastníka/)
    expect(canChangeRole({ role: 'owner', isSelf: true }, owner)).toBe(true)
    expect(changeRoleBlockedReason({ role: 'admin', isSelf: false }, owner)).toBeNull()
  })
})

describe('canRemove', () => {
  it('anyone may leave, except the last owner', () => {
    expect(canRemove({ role: 'member', isSelf: true }, member)).toBe(true)
    expect(canRemove({ role: 'accountant', isSelf: true }, { actor: 'accountant', ownerCount: 1 })).toBe(true)
    expect(canRemove({ role: 'owner', isSelf: true }, { actor: 'owner', ownerCount: 1 })).toBe(false)
    expect(canRemove({ role: 'owner', isSelf: true }, owner)).toBe(true)
  })
  it('removing others needs a manager, owners only by an owner', () => {
    expect(canRemove({ role: 'member', isSelf: false }, member)).toBe(false)
    expect(canRemove({ role: 'member', isSelf: false }, admin)).toBe(true)
    expect(canRemove({ role: 'owner', isSelf: false }, { actor: 'admin', ownerCount: 2 })).toBe(false)
    expect(canRemove({ role: 'owner', isSelf: false }, owner)).toBe(true)
  })
})

describe('canManageInvitation', () => {
  it('follows the same owner rule', () => {
    expect(canManageInvitation('member', 'admin')).toBe(true)
    expect(canManageInvitation('owner', 'admin')).toBe(false)
    expect(canManageInvitation('owner', 'owner')).toBe(true)
    expect(canManageInvitation('member', 'member')).toBe(false)
  })
})

describe('teamErrorMessage', () => {
  it('translates backend problems', () => {
    expect(teamErrorMessage(new ApiError(403, { code: 'owner_only' }))).toBe(
      'Vlastníky může spravovat jen jiný vlastník.',
    )
    expect(teamErrorMessage(new ApiError(409, { code: 'last_owner' }))).toBe(
      'Účet musí mít alespoň jednoho vlastníka.',
    )
    expect(teamErrorMessage(new ApiError(409, { code: 'already_member' }))).toBe(
      'Tento uživatel už je členem účtu.',
    )
    expect(teamErrorMessage(new ApiError(409, { code: 'already_joined' }))).toBe(
      'Už jste členem tohoto účtu.',
    )
    expect(teamErrorMessage(new ApiError(410, {}))).toMatch(/vypršela/)
    expect(teamErrorMessage(new ApiError(502, { detail: 'failed to send the invitation e-mail' }))).toMatch(/odeslat/)
    expect(teamErrorMessage(new ApiError(422, { errors: [{ location: 'body.email' }] }))).toBe('Zadejte platný e-mail.')
    expect(teamErrorMessage(new Error('x'))).toBe('Nastala neočekávaná chyba.')
  })
})
