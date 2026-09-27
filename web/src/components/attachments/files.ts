// Čisté helpery pro přílohy (validace na klientu, druh souboru, velikost).

import { LOCALE } from '@/lib/money'

/** Limit backendu (SPEC §7.12). */
export const MAX_ATTACHMENT_BYTES = 20 * 1024 * 1024

/** `accept` pro file input: obrázky, PDF a XML (ISDOC apod.). */
export const ATTACHMENT_ACCEPT = 'image/*,application/pdf,.pdf,.xml,.isdoc,text/xml,application/xml,.heic,.heif'

/** `accept` pro focení dokladu na mobilu. */
export const RECEIPT_CAPTURE_ACCEPT = 'image/*,application/pdf'

const ALLOWED_EXT = /\.(pdf|png|jpe?g|webp|heic|heif|xml|isdoc)$/i

/** Chybová hláška pro nepovolený soubor, nebo `null`. Typ ověřuje definitivně až backend (podle obsahu). */
export function validateAttachmentFile(file: { name: string; size: number; type: string }): string | null {
  if (file.size === 0) return `Soubor „${file.name}“ je prázdný.`
  if (file.size > MAX_ATTACHMENT_BYTES) {
    return `Soubor „${file.name}“ je příliš velký (${formatFileSize(file.size)}). Maximum je 20 MB.`
  }
  const typeOk =
    file.type.startsWith('image/') ||
    file.type === 'application/pdf' ||
    file.type === 'text/xml' ||
    file.type === 'application/xml' ||
    ALLOWED_EXT.test(file.name)
  if (!typeOk) return `Soubor „${file.name}“ nelze nahrát. Povolené jsou obrázky, PDF a XML.`
  return null
}

export type AttachmentKind = 'image' | 'pdf' | 'xml' | 'other'

/** Druh souboru podle content type (fallback na příponu). HEIC se jako obrázek nezobrazí všude. */
export function attachmentKind(contentType: string, filename = ''): AttachmentKind {
  const ct = contentType.toLowerCase()
  if (ct.startsWith('image/') || /\.(png|jpe?g|webp|gif|heic|heif)$/i.test(filename)) return 'image'
  if (ct === 'application/pdf' || /\.pdf$/i.test(filename)) return 'pdf'
  if (ct.includes('xml') || /\.(xml|isdoc)$/i.test(filename)) return 'xml'
  return 'other'
}

/** Prohlížeč umí obrázek vykreslit (HEIC typicky ne). */
export function isPreviewableImage(contentType: string): boolean {
  return /^image\/(png|jpe?g|webp|gif|avif|svg\+xml)$/i.test(contentType)
}

/** Česká hláška pro známé chyby uploadu (backend vrací anglický `detail`). */
export function uploadErrorText(status: number): string | null {
  if (status === 413) return 'Soubor je větší než 20 MB.'
  if (status === 415) return 'Nepodporovaný typ souboru. Povolené jsou PDF, obrázky a XML.'
  return null
}

const sizeFmt = new Intl.NumberFormat(LOCALE, { maximumFractionDigits: 1 })

/** `1536` → `"1,5 kB"`, `2_500_000` → `"2,4 MB"`. */
export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${sizeFmt.format(bytes / 1024)} kB`
  return `${sizeFmt.format(bytes / (1024 * 1024))} MB`
}
