import { notFound } from '@tanstack/react-router'

/**
 * Parsování číselného ID z path parametru (`params.parse` v route).
 * Neplatné ID → 404 stránka.
 */
export function parseId(raw: string): number {
  const id = Number(raw)
  if (!/^\d+$/.test(raw) || !Number.isSafeInteger(id) || id <= 0) throw notFound()
  return id
}
