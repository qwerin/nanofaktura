import { describe, expect, it } from 'vitest'
import { attachmentKind, formatFileSize, isPreviewableImage, MAX_ATTACHMENT_BYTES, uploadErrorText, validateAttachmentFile } from './files'

describe('validateAttachmentFile', () => {
  it('accepts images, PDF and XML up to 20 MB', () => {
    expect(validateAttachmentFile({ name: 'a.jpg', size: 1000, type: 'image/jpeg' })).toBeNull()
    expect(validateAttachmentFile({ name: 'a.pdf', size: MAX_ATTACHMENT_BYTES, type: 'application/pdf' })).toBeNull()
    expect(validateAttachmentFile({ name: 'faktura.isdoc', size: 10, type: '' })).toBeNull()
    expect(validateAttachmentFile({ name: 'IMG_1.HEIC', size: 10, type: '' })).toBeNull()
  })

  it('rejects too big, empty and unsupported files', () => {
    expect(validateAttachmentFile({ name: 'big.pdf', size: MAX_ATTACHMENT_BYTES + 1, type: 'application/pdf' })).toMatch(/20 MB/)
    expect(validateAttachmentFile({ name: 'x.pdf', size: 0, type: 'application/pdf' })).toMatch(/prázdný/)
    expect(validateAttachmentFile({ name: 'x.docx', size: 10, type: 'application/vnd.openxmlformats' })).toMatch(/nelze/)
  })
})

describe('attachmentKind', () => {
  it('detects kinds by content type and extension', () => {
    expect(attachmentKind('image/png')).toBe('image')
    expect(attachmentKind('application/octet-stream', 'scan.JPG')).toBe('image')
    expect(attachmentKind('application/pdf')).toBe('pdf')
    expect(attachmentKind('application/xml')).toBe('xml')
    expect(attachmentKind('text/plain', 'a.txt')).toBe('other')
  })

  it('knows which images the browser can preview', () => {
    expect(isPreviewableImage('image/webp')).toBe(true)
    expect(isPreviewableImage('image/heic')).toBe(false)
  })
})

describe('formatting', () => {
  it('formats file sizes in Czech', () => {
    expect(formatFileSize(512)).toBe('512 B')
    expect(formatFileSize(1536)).toBe('1,5 kB')
    expect(formatFileSize(2.5 * 1024 * 1024)).toBe('2,5 MB')
  })

  it('maps upload errors', () => {
    expect(uploadErrorText(413)).toMatch(/20 MB/)
    expect(uploadErrorText(415)).toMatch(/typ/)
    expect(uploadErrorText(500)).toBeNull()
  })
})
