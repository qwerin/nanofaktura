// Stavba URL exportů (§7.11) z filtrů seznamu — čisté funkce, testované v export-url.test.ts.
//
//   exportUrl('acme', 'invoices', 'csv', { status: 'paid' }) → /api/accounts/acme/exports/invoices.csv?status=paid

export type ExportKind = 'invoices' | 'subjects' | 'expenses'
export type ExportFormat = 'csv' | 'xlsx'

/** Hodnoty filtrů: prázdné (`undefined`, `null`, `''`, `false`) se do URL nepíšou. */
export type ExportFilters = Record<string, string | number | boolean | null | undefined>

function base(): string {
  return import.meta.env.VITE_API_BASE_URL ?? ''
}

/** Query string se stabilním pořadím klíčů (abecedně), bez prázdných hodnot; s `?`, nebo prázdný. */
export function exportQuery(filters: ExportFilters = {}): string {
  const params = new URLSearchParams()
  for (const key of Object.keys(filters).sort()) {
    const v = filters[key]
    if (v === undefined || v === null || v === '' || v === false) continue
    const s = typeof v === 'string' ? v.trim() : String(v)
    if (s === '') continue
    params.set(key, s)
  }
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

export function exportUrl(slug: string, kind: ExportKind, format: ExportFormat, filters?: ExportFilters): string {
  return `${base()}/api/accounts/${encodeURIComponent(slug)}/exports/${kind}.${format}${exportQuery(filters)}`
}

/** ZIP s PDF (a volitelně ISDOC) faktur podle stejných filtrů jako seznam. */
export function pdfZipUrl(slug: string, filters?: ExportFilters, opts: { isdoc?: boolean } = {}): string {
  return `${base()}/api/accounts/${encodeURIComponent(slug)}/exports/pdf.zip${exportQuery({ ...filters, isdoc: opts.isdoc })}`
}

/** Název souboru z `Content-Disposition` (RFC 6266, včetně `filename*=UTF-8''…`), jinak `fallback`. */
export function filenameFromDisposition(header: string | null | undefined, fallback: string): string {
  if (!header) return fallback
  const star = /filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/.exec(header)
  if (star?.[1]) {
    try {
      return decodeURIComponent(star[1].trim().replace(/^"|"$/g, ''))
    } catch {
      /* špatné kódování → zkusit obyčejné filename */
    }
  }
  const plain = /filename\s*=\s*("([^"]*)"|[^;]+)/.exec(header)
  const name = (plain?.[2] ?? plain?.[1] ?? '').trim()
  return name || fallback
}
