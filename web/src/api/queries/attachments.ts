import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { ApiError, type Problem } from '../errors'
import type { Attachment, AttachmentOwnerType } from '../types'
import { keys } from './keys'

export const attachmentQueries = {
  /** Přílohy jednoho záznamu (bez stránkování v UI — načte až 200 kusů). */
  list: (slug: string, ownerType: AttachmentOwnerType, ownerId: number) =>
    queryOptions({
      queryKey: keys.attachmentList(slug, ownerType, ownerId),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/attachments', {
              params: { path: { slug }, query: { owner_type: ownerType, owner_id: ownerId, per_page: 200 } },
              signal,
            }),
          )
        ).items ?? [],
    }),
}

/** URL pro stažení (`inline` = otevřít v prohlížeči / náhled obrázku). */
export function attachmentUrl(slug: string, id: number, opts: { inline?: boolean } = {}): string {
  const base = import.meta.env.VITE_API_BASE_URL ?? ''
  const path = `/api/accounts/${encodeURIComponent(slug)}/attachments/${id}/download`
  return `${base}${path}${opts.inline ? '?inline=true' : ''}`
}

export interface UploadAttachmentInput {
  ownerType: AttachmentOwnerType
  ownerId: number
  file: File
  /** Průběh odesílání 0–1. */
  onProgress?: (fraction: number) => void
  signal?: AbortSignal
}

/**
 * Nahrání přílohy (multipart). Přes XMLHttpRequest kvůli průběhu odesílání — fetch ho neumí.
 * Chyby převádí na `ApiError` stejně jako `unwrap`.
 */
export function uploadAttachment(slug: string, input: UploadAttachmentInput): Promise<Attachment> {
  const { ownerType, ownerId, file, onProgress, signal } = input
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const base = import.meta.env.VITE_API_BASE_URL ?? ''
    xhr.open('POST', `${base}/api/accounts/${encodeURIComponent(slug)}/attachments`)
    xhr.withCredentials = true
    xhr.setRequestHeader('Accept', 'application/json, application/problem+json')
    xhr.responseType = 'json'

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded / e.total)
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.(1)
        resolve(xhr.response as Attachment)
        return
      }
      const body: unknown = xhr.response
      const problem: Problem =
        body && typeof body === 'object' ? (body as Problem) : { title: xhr.statusText || `HTTP ${xhr.status}` }
      reject(new ApiError(xhr.status, problem))
    }
    xhr.onerror = () => reject(new ApiError(0, { title: 'Network error' }))
    xhr.onabort = () => reject(new DOMException('Upload aborted', 'AbortError'))
    signal?.addEventListener('abort', () => xhr.abort(), { once: true })

    const form = new FormData()
    form.append('owner_type', ownerType)
    form.append('owner_id', String(ownerId))
    form.append('file', file, file.name)
    xhr.send(form)
  })
}

export function useUploadAttachment(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UploadAttachmentInput) => uploadAttachment(slug, input),
    onSuccess: (att) => {
      void qc.invalidateQueries({ queryKey: keys.attachmentList(slug, att.owner_type, att.owner_id) })
    },
    // Chyby (vč. zrušení) řeší volající — u více souborů najednou chce vědět, který selhal.
    meta: { silent: true },
  })
}

export function useDeleteAttachment(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (att: Attachment) =>
      unwrap(api.DELETE('/api/accounts/{slug}/attachments/{id}', { params: { path: { slug, id: att.id } } })),
    onSuccess: (_data, att) => {
      qc.setQueryData<Attachment[]>(keys.attachmentList(slug, att.owner_type, att.owner_id), (prev) =>
        prev?.filter((a) => a.id !== att.id),
      )
      void qc.invalidateQueries({ queryKey: keys.attachmentList(slug, att.owner_type, att.owner_id) })
    },
  })
}
