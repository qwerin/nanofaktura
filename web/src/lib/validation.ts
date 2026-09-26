// Sdílená validační pravidla pro formuláře (Zod). Hlášky česky.

import { z } from 'zod'

export const MIN_PASSWORD_LENGTH = 8

export const passwordSchema = z
  .string()
  .min(MIN_PASSWORD_LENGTH, `Heslo musí mít aspoň ${MIN_PASSWORD_LENGTH} znaků`)

/** IČO: 8 číslic s kontrolním součtem (mod 11). Kratší IČO se doplní nulami zleva. */
export function isValidIco(value: string): boolean {
  if (!/^\d{1,8}$/.test(value)) return false
  const ico = value.padStart(8, '0')
  let sum = 0
  for (let i = 0; i < 7; i++) sum += Number(ico[i]) * (8 - i)
  const check = (11 - (sum % 11)) % 10
  return check === Number(ico[7])
}

/** Volitelné IČO (prázdný string = nevyplněno). */
export const optionalIcoSchema = z
  .string()
  .trim()
  .refine((v) => v === '' || isValidIco(v), 'Neplatné IČO')

/** Volitelný e-mail (prázdný string = nevyplněno). */
export const optionalEmailSchema = z
  .string()
  .trim()
  .refine((v) => v === '' || z.email().safeParse(v).success, 'Neplatný e-mail')
