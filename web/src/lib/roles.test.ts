import { describe, expect, it } from 'vitest'
import { canEditRole, canManageSettingsRole, roleLabel } from './roles'

describe('roles', () => {
  it('settings are for owner and admin', () => {
    expect(canManageSettingsRole('owner')).toBe(true)
    expect(canManageSettingsRole('admin')).toBe(true)
    expect(canManageSettingsRole('member')).toBe(false)
    expect(canManageSettingsRole('accountant')).toBe(false)
    expect(canManageSettingsRole(undefined)).toBe(false)
  })
  it('editing is for everyone except accountant', () => {
    expect(canEditRole('owner')).toBe(true)
    expect(canEditRole('admin')).toBe(true)
    expect(canEditRole('member')).toBe(true)
    expect(canEditRole('accountant')).toBe(false)
  })
  it('has Czech labels', () => {
    expect(roleLabel('accountant')).toBe('Účetní')
    expect(roleLabel(undefined)).toBe('')
  })
})
