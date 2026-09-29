// Stažení souboru z API přes fetch → blob (session cookie, čitelné chyby problem+json, spinner).
// Obyčejný <a download> by při chybě uložil JSON s chybou jako soubor.

import { ApiError, type Problem } from '@/api/errors'
import { filenameFromDisposition } from './export-url'

/** Stáhne `url` a uloží ho pod názvem z `Content-Disposition` (nebo `fallbackName`). Vyhazuje `ApiError`. */
export async function downloadFile(url: string, fallbackName: string, signal?: AbortSignal): Promise<string> {
  let res: Response
  try {
    res = await fetch(url, { credentials: 'include', signal })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, { title: 'Network error' })
  }
  if (!res.ok) {
    let problem: Problem = { title: res.statusText }
    try {
      const body: unknown = await res.json()
      if (body && typeof body === 'object') problem = body as Problem
    } catch {
      /* tělo není JSON */
    }
    throw new ApiError(res.status, problem)
  }
  const blob = await res.blob()
  const name = filenameFromDisposition(res.headers.get('Content-Disposition'), fallbackName)
  saveBlob(blob, name)
  return name
}

export function saveBlob(blob: Blob, name: string) {
  const href = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = href
  a.download = name
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Safari potřebuje URL ještě chvíli po kliknutí
  setTimeout(() => URL.revokeObjectURL(href), 30_000)
}
