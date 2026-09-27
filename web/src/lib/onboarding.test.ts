import { describe, expect, it } from 'vitest'
import { isProfileEmpty } from './onboarding'

describe('isProfileEmpty', () => {
  const empty = { registration_no: '', street: '', city: '', email: '' }
  it('is empty right after registration', () => expect(isProfileEmpty(empty)).toBe(true))
  it('is filled once any profile field is set', () => {
    expect(isProfileEmpty({ ...empty, registration_no: '27074358' })).toBe(false)
    expect(isProfileEmpty({ ...empty, city: 'Praha' })).toBe(false)
  })
})
