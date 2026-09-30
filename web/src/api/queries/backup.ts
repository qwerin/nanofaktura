// Záloha a obnova účtu (SPEC §7.16). Stažení i nahrání jdou mimo openapi-fetch,
// protože potřebují průběh: stažení čte stream po částech, nahrání přes XHR (upload progress).

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { saveBlob } from '@/components/export/download'
import { filenameFromDisposition } from '@/components/export/export-url'
import { ApiError, type Problem } from '../errors'
import type { ResponseBody } from '../types'
import { keys } from './keys'

export type ImportResult = ResponseBody<'/api/accounts/import', 'post'>
export type ImportWarning = ImportResult['warnings'][number]

const baseUrl: string = import.meta.env.VITE_API_BASE_URL ?? ''

async function problemOf(res: Response): Promise<ApiError> {
  let problem: Problem = { title: res.statusText }
  try {
    const body: unknown = await res.json()
    if (body && typeof body === 'object') problem = body as Problem
  } catch {
    /* tělo není JSON */
  }
  return new ApiError(res.status, problem)
}

/**
 * Stáhne zálohu účtu (ZIP) a uloží ji. `onProgress(bytes)` hlásí stažené bajty
 * (server zálohu streamuje, celková velikost předem známá není). Vrací název souboru a velikost.
 */
export async function downloadBackup(
  slug: string,
  onProgress?: (loaded: number) => void,
  signal?: AbortSignal,
): Promise<{ name: string; size: number }> {
  let res: Response
  try {
    res = await fetch(`${baseUrl}/api/accounts/${encodeURIComponent(slug)}/backup`, { credentials: 'include', signal })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, { title: 'Network error' })
  }
  if (!res.ok) throw await problemOf(res)
  const chunks: Uint8Array[] = []
  let loaded = 0
  const reader = res.body?.getReader()
  if (reader) {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      chunks.push(value)
      loaded += value.byteLength
      onProgress?.(loaded)
    }
  } else {
    const buf = new Uint8Array(await res.arrayBuffer())
    chunks.push(buf)
    loaded = buf.byteLength
  }
  const name = filenameFromDisposition(res.headers.get('Content-Disposition'), `nanofaktura-${slug}.zip`)
  saveBlob(new Blob(chunks as BlobPart[], { type: 'application/zip' }), name)
  return { name, size: loaded }
}

/** Nahraje zálohu a založí z ní nový účet. `onProgress(loaded, total)` = průběh nahrávání. */
export function uploadBackup(
  file: File,
  name: string,
  onProgress?: (loaded: number, total: number) => void,
): Promise<ImportResult> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${baseUrl}/api/accounts/import`)
    xhr.withCredentials = true
    xhr.setRequestHeader('Accept', 'application/json, application/problem+json')
    xhr.upload.onprogress = (e) => onProgress?.(e.loaded, e.lengthComputable ? e.total : file.size)
    xhr.onerror = () => reject(new ApiError(0, { title: 'Network error' }))
    xhr.onload = () => {
      let body: unknown = null
      try {
        body = JSON.parse(xhr.responseText)
      } catch {
        /* není JSON */
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(body as ImportResult)
      } else {
        const problem = body && typeof body === 'object' ? (body as Problem) : { title: xhr.statusText }
        reject(new ApiError(xhr.status, problem))
      }
    }
    const form = new FormData()
    form.append('file', file)
    if (name.trim()) form.append('name', name.trim())
    xhr.send(form)
  })
}

/** Obnova ze zálohy; po úspěchu obnoví seznam účtů (přepínač čte /api/auth/me). */
export function useImportBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ file, name, onProgress }: { file: File; name: string; onProgress?: (loaded: number, total: number) => void }) =>
      uploadBackup(file, name, onProgress),
    onSuccess: async (result) => {
      qc.setQueryData(keys.accountDetail(result.account.slug), result.account)
      await Promise.all([qc.invalidateQueries({ queryKey: keys.me() }), qc.invalidateQueries({ queryKey: keys.accounts() })])
    },
    meta: { silent: true },
  })
}
