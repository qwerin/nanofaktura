import { describe, expect, it } from 'vitest'
import { BRANDING_MAX_BYTES, validateBrandingFile } from './branding-api'

describe('validateBrandingFile', () => {
  it('accepts PNG and JPEG', () => {
    expect(validateBrandingFile({ type: 'image/png', size: 1000, name: 'logo.png' })).toBeNull()
    expect(validateBrandingFile({ type: 'image/jpeg', size: 1000, name: 'podpis.jpg' })).toBeNull()
    // bez MIME typu (některé mobilní prohlížeče) rozhoduje přípona
    expect(validateBrandingFile({ type: '', size: 1000, name: 'LOGO.JPEG' })).toBeNull()
  })
  it('rejects other types, empty and too large files', () => {
    expect(validateBrandingFile({ type: 'image/webp', size: 1000, name: 'a.webp' })).toMatch(/PNG nebo JPEG/)
    expect(validateBrandingFile({ type: 'application/pdf', size: 1000, name: 'a.pdf' })).toMatch(/PNG nebo JPEG/)
    expect(validateBrandingFile({ type: '', size: 1000, name: 'a.heic' })).toMatch(/PNG nebo JPEG/)
    expect(validateBrandingFile({ type: 'image/png', size: BRANDING_MAX_BYTES + 1, name: 'a.png' })).toMatch(/velký/)
    expect(validateBrandingFile({ type: 'image/png', size: 0, name: 'a.png' })).toMatch(/prázdný/)
  })
})
