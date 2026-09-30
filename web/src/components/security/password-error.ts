import type { FieldValues, Path, UseFormSetError } from 'react-hook-form'
import { codeMessages, errorMessage, hasErrorCode } from '@/api/errors'

/**
 * Chyba citlivé akce → formulář: špatné heslo (422 `wrong_password`, `body.password`) k poli hesla,
 * ostatní do `root`. Vrací `true`, když šlo o špatné heslo.
 */
export function passwordErrorToForm<T extends FieldValues & { password: string }>(
  err: unknown,
  setError: UseFormSetError<T>,
): boolean {
  if (hasErrorCode(err, 'wrong_password')) {
    setError('password' as Path<T>, { type: 'server', message: codeMessages.wrong_password })
    return true
  }
  setError('root', { type: 'server', message: errorMessage(err) })
  return false
}
