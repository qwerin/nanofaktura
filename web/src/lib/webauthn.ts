// Bezpečnostní klíče (WebAuthn, SPEC §3.1): převod JSON voleb z backendu na volby pro
// navigator.credentials.create/get a zpět výsledného PublicKeyCredential na JSON (base64url).
//
// Backend posílá `PublicKeyCredentialCreationOptions` / `…RequestOptions` jako JSON s binárními poli
// v base64url. Moderní prohlížeče umí `PublicKeyCredential.parse…FromJSON` a `credential.toJSON()`;
// pro ostatní je tu ruční převod.

/** Binární data → base64url bez paddingu. */
export function base64urlEncode(data: ArrayBuffer | ArrayBufferView): string {
  const bytes =
    data instanceof ArrayBuffer ? new Uint8Array(data) : new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/** base64url (i s paddingem nebo v klasickém base64) → ArrayBuffer. */
export function base64urlDecode(value: string): ArrayBuffer {
  const b64 = value.replace(/-/g, '+').replace(/_/g, '/').replace(/=+$/, '')
  const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4))
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return bytes.buffer
}

type JsonObject = Record<string, unknown>

function asObject(value: unknown): JsonObject {
  return value && typeof value === 'object' ? (value as JsonObject) : {}
}

function decodeIfString(value: unknown): unknown {
  return typeof value === 'string' ? base64urlDecode(value) : value
}

function decodeDescriptors(list: unknown): unknown {
  if (!Array.isArray(list)) return list
  return list.map((d) => {
    const o = asObject(d)
    return { ...o, id: decodeIfString(o.id) }
  })
}

/** Ruční převod JSON → PublicKeyCredentialCreationOptions (challenge, user.id, excludeCredentials[].id). */
export function decodeCreationOptions(json: unknown): PublicKeyCredentialCreationOptions {
  const o = asObject(json)
  const user = asObject(o.user)
  const out: JsonObject = {
    ...o,
    challenge: decodeIfString(o.challenge),
    user: { ...user, id: decodeIfString(user.id) },
  }
  if (o.excludeCredentials !== undefined) out.excludeCredentials = decodeDescriptors(o.excludeCredentials)
  return out as unknown as PublicKeyCredentialCreationOptions
}

/** Ruční převod JSON → PublicKeyCredentialRequestOptions (challenge, allowCredentials[].id). */
export function decodeRequestOptions(json: unknown): PublicKeyCredentialRequestOptions {
  const o = asObject(json)
  const out: JsonObject = { ...o, challenge: decodeIfString(o.challenge) }
  if (o.allowCredentials !== undefined) out.allowCredentials = decodeDescriptors(o.allowCredentials)
  return out as unknown as PublicKeyCredentialRequestOptions
}

/** Minimum z PublicKeyCredential, které serializujeme (umožňuje testy bez prohlížeče). */
export interface CredentialLike {
  id: string
  rawId: ArrayBuffer
  type: string
  authenticatorAttachment?: string | null
  response: {
    clientDataJSON: ArrayBuffer
    attestationObject?: ArrayBuffer
    authenticatorData?: ArrayBuffer
    signature?: ArrayBuffer
    userHandle?: ArrayBuffer | null
    getTransports?: () => string[]
  }
  getClientExtensionResults?: () => unknown
}

export interface CredentialJSON {
  id: string
  rawId: string
  type: string
  authenticatorAttachment?: string
  response: Record<string, string | string[]>
  clientExtensionResults: unknown
}

/** Ruční serializace výsledku navigator.credentials.create/get do JSON (base64url). */
export function encodeCredential(cred: CredentialLike): CredentialJSON {
  const r = cred.response
  const response: Record<string, string | string[]> = { clientDataJSON: base64urlEncode(r.clientDataJSON) }
  if (r.attestationObject) {
    response.attestationObject = base64urlEncode(r.attestationObject)
    const transports = typeof r.getTransports === 'function' ? r.getTransports() : undefined
    if (transports?.length) response.transports = transports
  }
  if (r.authenticatorData) response.authenticatorData = base64urlEncode(r.authenticatorData)
  if (r.signature) response.signature = base64urlEncode(r.signature)
  if (r.userHandle && r.userHandle.byteLength > 0) response.userHandle = base64urlEncode(r.userHandle)
  const out: CredentialJSON = {
    id: cred.id,
    rawId: base64urlEncode(cred.rawId),
    type: cred.type,
    response,
    clientExtensionResults:
      typeof cred.getClientExtensionResults === 'function' ? cred.getClientExtensionResults() : {},
  }
  if (cred.authenticatorAttachment) out.authenticatorAttachment = cred.authenticatorAttachment
  return out
}

type PkcStatic = {
  parseCreationOptionsFromJSON?: (o: unknown) => PublicKeyCredentialCreationOptions
  parseRequestOptionsFromJSON?: (o: unknown) => PublicKeyCredentialRequestOptions
}

function pkcStatic(): PkcStatic | undefined {
  return typeof window !== 'undefined' && 'PublicKeyCredential' in window
    ? (window.PublicKeyCredential as unknown as PkcStatic)
    : undefined
}

function toJSON(cred: Credential | null): unknown {
  if (!cred) throw new DOMException('No credential', 'NotAllowedError')
  const withJSON = cred as Credential & { toJSON?: () => unknown }
  if (typeof withJSON.toJSON === 'function') {
    try {
      return withJSON.toJSON()
    } catch {
      /* některé implementace toJSON selžou — ruční převod níže */
    }
  }
  return encodeCredential(cred as unknown as CredentialLike)
}

/** Podpora v prohlížeči: `ok`, `insecure` (stránka není HTTPS/localhost) nebo `unsupported`. */
export type WebAuthnSupport = 'ok' | 'insecure' | 'unsupported'

export function webauthnSupport(): WebAuthnSupport {
  if (typeof window === 'undefined') return 'unsupported'
  if (!window.isSecureContext) return 'insecure'
  if (!('PublicKeyCredential' in window) || !navigator.credentials) return 'unsupported'
  return 'ok'
}

/** Česká nápověda, proč klíč nejde použít (nebo `null`, když jde). */
export function webauthnSupportHint(support: WebAuthnSupport = webauthnSupport()): string | null {
  switch (support) {
    case 'insecure':
      return 'Bezpečnostní klíče fungují jen na zabezpečeném připojení (https://). Požádejte správce instance o HTTPS.'
    case 'unsupported':
      return 'Tento prohlížeč bezpečnostní klíče nepodporuje. Zkuste aktuální Chrome, Edge, Firefox nebo Safari.'
    default:
      return null
  }
}

/** Registrace nového klíče: JSON volby z backendu → prohlížeč → JSON pro backend. */
export async function createCredential(optionsJSON: unknown): Promise<unknown> {
  const pkc = pkcStatic()
  let publicKey: PublicKeyCredentialCreationOptions
  try {
    publicKey = pkc?.parseCreationOptionsFromJSON
      ? pkc.parseCreationOptionsFromJSON(optionsJSON)
      : decodeCreationOptions(optionsJSON)
  } catch {
    publicKey = decodeCreationOptions(optionsJSON)
  }
  return toJSON(await navigator.credentials.create({ publicKey }))
}

/** Přihlášení klíčem: JSON volby z backendu → prohlížeč → JSON pro backend. */
export async function getCredential(optionsJSON: unknown): Promise<unknown> {
  const pkc = pkcStatic()
  let publicKey: PublicKeyCredentialRequestOptions
  try {
    publicKey = pkc?.parseRequestOptionsFromJSON
      ? pkc.parseRequestOptionsFromJSON(optionsJSON)
      : decodeRequestOptions(optionsJSON)
  } catch {
    publicKey = decodeRequestOptions(optionsJSON)
  }
  return toJSON(await navigator.credentials.get({ publicKey }))
}

/** Uživatel dialog prohlížeče zavřel / nechal vypršet — nic strašného, stačí zkusit znovu. */
export function isWebAuthnCancelled(err: unknown): boolean {
  return err instanceof DOMException && (err.name === 'NotAllowedError' || err.name === 'AbortError')
}

/** Česká hláška k chybě prohlížeče při práci s klíčem (`null` = uživatel ověření zrušil). */
export function webauthnErrorMessage(err: unknown): string | null {
  if (isWebAuthnCancelled(err)) return null
  if (err instanceof DOMException) {
    switch (err.name) {
      case 'InvalidStateError':
        return 'Tento klíč už je přidaný.'
      case 'SecurityError':
        return 'Prohlížeč odmítl klíč pro tuto adresu. Klíče fungují jen na adrese, pod kterou instance běží (a přes HTTPS).'
      case 'NotSupportedError':
        return 'Klíč nebo prohlížeč tento způsob ověření nepodporuje.'
    }
  }
  return 'Bezpečnostní klíč se nepodařilo použít. Zkuste to znovu.'
}
