import { describe, expect, it } from 'vitest'
import { exportQuery, exportUrl, filenameFromDisposition, pdfZipUrl } from './export-url'

describe('exportQuery', () => {
  it('drops empty values and sorts keys', () => {
    expect(exportQuery({ until: '2026-12-31', status: 'paid', query: '  ', since: undefined, isdoc: false })).toBe(
      '?status=paid&until=2026-12-31',
    )
  })
  it('returns empty string without filters', () => {
    expect(exportQuery()).toBe('')
    expect(exportQuery({ a: undefined, b: null, c: '' })).toBe('')
  })
  it('encodes values', () => {
    expect(exportQuery({ query: 'Nováček & syn' })).toBe('?query=Nov%C3%A1%C4%8Dek+%26+syn')
    expect(exportQuery({ subject_id: 12, isdoc: true })).toBe('?isdoc=true&subject_id=12')
  })
})

describe('export URLs', () => {
  it('builds list exports', () => {
    expect(exportUrl('acme', 'invoices', 'csv', { status: 'overdue' })).toBe(
      '/api/accounts/acme/exports/invoices.csv?status=overdue',
    )
    expect(exportUrl('a b', 'subjects', 'xlsx', { type: 'customer' })).toBe(
      '/api/accounts/a%20b/exports/subjects.xlsx?type=customer',
    )
  })
  it('builds the PDF zip with an optional isdoc flag', () => {
    expect(pdfZipUrl('acme', { since: '2026-01-01' })).toBe('/api/accounts/acme/exports/pdf.zip?since=2026-01-01')
    expect(pdfZipUrl('acme', {}, { isdoc: true })).toBe('/api/accounts/acme/exports/pdf.zip?isdoc=true')
  })
})

describe('filenameFromDisposition', () => {
  it('reads quoted and bare names', () => {
    expect(filenameFromDisposition('attachment; filename="faktury.csv"', 'x')).toBe('faktury.csv')
    expect(filenameFromDisposition('attachment; filename=faktury.xlsx', 'x')).toBe('faktury.xlsx')
  })
  it('prefers RFC 5987 filename*', () => {
    expect(filenameFromDisposition(`attachment; filename="a.zip"; filename*=UTF-8''fakt%C5%AFry.zip`, 'x')).toBe(
      'faktůry.zip',
    )
  })
  it('falls back', () => {
    expect(filenameFromDisposition(null, 'export.csv')).toBe('export.csv')
    expect(filenameFromDisposition('inline', 'export.csv')).toBe('export.csv')
  })
})
