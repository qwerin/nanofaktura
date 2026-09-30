import { describe, expect, it } from 'vitest'
import {
  base64urlDecode,
  base64urlEncode,
  decodeCreationOptions,
  decodeRequestOptions,
  encodeCredential,
  isWebAuthnCancelled,
  webauthnErrorMessage,
} from './webauthn'

const bytes = (...b: number[]) => new Uint8Array(b).buffer

describe('base64url', () => {
  it('encodes without padding and with url-safe alphabet', () => {
    expect(base64urlEncode(bytes(0xfb, 0xff))).toBe('-_8')
    expect(base64urlEncode(new TextEncoder().encode('hello'))).toBe('aGVsbG8')
    expect(base64urlEncode(bytes())).toBe('')
  })
  it('respects the view offset of typed arrays', () => {
    const all = new Uint8Array([1, 2, 3, 4])
    expect(base64urlEncode(all.subarray(1, 3))).toBe(base64urlEncode(bytes(2, 3)))
  })
  it('decodes unpadded, padded and standard base64', () => {
    expect(new Uint8Array(base64urlDecode('-_8'))).toEqual(new Uint8Array([0xfb, 0xff]))
    expect(new Uint8Array(base64urlDecode('+/8='))).toEqual(new Uint8Array([0xfb, 0xff]))
    expect(new TextDecoder().decode(base64urlDecode('aGVsbG8'))).toBe('hello')
  })
  it('roundtrips random data', () => {
    for (let n = 0; n < 40; n++) {
      const data = new Uint8Array(n).map((_, i) => (i * 37 + n) & 0xff)
      expect(new Uint8Array(base64urlDecode(base64urlEncode(data)))).toEqual(data)
    }
  })
})

describe('options decoding', () => {
  it('decodes binary fields of creation options', () => {
    const o = decodeCreationOptions({
      rp: { id: 'localhost', name: 'NanoFaktura' },
      user: { id: 'AQI', name: 'a@example.cz', displayName: 'A' },
      challenge: 'AwQF',
      pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
      excludeCredentials: [{ type: 'public-key', id: 'Bgc', transports: ['usb'] }],
    })
    expect(new Uint8Array(o.challenge as ArrayBuffer)).toEqual(new Uint8Array([3, 4, 5]))
    expect(new Uint8Array(o.user.id as ArrayBuffer)).toEqual(new Uint8Array([1, 2]))
    expect(o.user.name).toBe('a@example.cz')
    expect(o.rp.id).toBe('localhost')
    expect(new Uint8Array(o.excludeCredentials![0]!.id as ArrayBuffer)).toEqual(new Uint8Array([6, 7]))
    expect(o.excludeCredentials![0]!.transports).toEqual(['usb'])
  })
  it('decodes binary fields of request options', () => {
    const o = decodeRequestOptions({ challenge: 'AwQF', rpId: 'localhost', allowCredentials: [{ type: 'public-key', id: 'Bgc' }] })
    expect(new Uint8Array(o.challenge as ArrayBuffer)).toEqual(new Uint8Array([3, 4, 5]))
    expect(o.rpId).toBe('localhost')
    expect(new Uint8Array(o.allowCredentials![0]!.id as ArrayBuffer)).toEqual(new Uint8Array([6, 7]))
  })
  it('keeps options without credential lists', () => {
    const o = decodeRequestOptions({ challenge: 'AwQF' })
    expect(o.allowCredentials).toBeUndefined()
  })
})

describe('encodeCredential', () => {
  it('serializes an attestation (registration)', () => {
    const json = encodeCredential({
      id: 'AQI',
      rawId: bytes(1, 2),
      type: 'public-key',
      authenticatorAttachment: 'cross-platform',
      response: { clientDataJSON: bytes(3), attestationObject: bytes(4, 5), getTransports: () => ['usb', 'nfc'] },
      getClientExtensionResults: () => ({ credProps: { rk: false } }),
    })
    expect(json).toEqual({
      id: 'AQI',
      rawId: 'AQI',
      type: 'public-key',
      authenticatorAttachment: 'cross-platform',
      response: { clientDataJSON: 'Aw', attestationObject: 'BAU', transports: ['usb', 'nfc'] },
      clientExtensionResults: { credProps: { rk: false } },
    })
  })
  it('serializes an assertion (login) and drops an empty user handle', () => {
    const json = encodeCredential({
      id: 'AQI',
      rawId: bytes(1, 2),
      type: 'public-key',
      authenticatorAttachment: null,
      response: { clientDataJSON: bytes(3), authenticatorData: bytes(6), signature: bytes(7), userHandle: bytes() },
    })
    expect(json).toEqual({
      id: 'AQI',
      rawId: 'AQI',
      type: 'public-key',
      response: { clientDataJSON: 'Aw', authenticatorData: 'Bg', signature: 'Bw' },
      clientExtensionResults: {},
    })
  })
})

describe('errors', () => {
  it('treats NotAllowedError as a quiet cancellation', () => {
    const err = new DOMException('cancelled', 'NotAllowedError')
    expect(isWebAuthnCancelled(err)).toBe(true)
    expect(webauthnErrorMessage(err)).toBeNull()
  })
  it('explains an already registered key', () => {
    const err = new DOMException('exists', 'InvalidStateError')
    expect(isWebAuthnCancelled(err)).toBe(false)
    expect(webauthnErrorMessage(err)).toMatch(/už je přidaný/)
  })
})
