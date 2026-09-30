import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '../client'
import { createCredential, getCredential } from '@/lib/webauthn'
import { isApiError } from '../errors'
import type { CreateTokenInput, LoginInput, Me, RegisterInput, UpdateMeInput } from '../types'
import { keys } from './keys'

// --- Queries (queryOptions → použitelné v komponentách i v route loaderech) ---

export const authQueries = {
  status: () =>
    queryOptions({
      queryKey: keys.authStatus(),
      queryFn: () => unwrap(api.GET('/api/auth/status')),
      staleTime: 5 * 60_000,
    }),

  /** Přihlášený uživatel, nebo `null` když session neexistuje (401). */
  me: () =>
    queryOptions({
      queryKey: keys.me(),
      queryFn: async (): Promise<Me | null> => {
        try {
          return await unwrap(api.GET('/api/auth/me'))
        } catch (err) {
          if (isApiError(err) && err.status === 401) return null
          throw err
        }
      },
      staleTime: 5 * 60_000,
      meta: { silent: true },
    }),

  twoFactor: () =>
    queryOptions({
      queryKey: keys.twoFactor(),
      queryFn: () => unwrap(api.GET('/api/auth/2fa')),
    }),

  passwordReset: (token: string) =>
    queryOptions({
      queryKey: keys.passwordReset(token),
      queryFn: () => unwrap(api.GET('/api/auth/password-reset/{token}', { params: { path: { token } } })),
      staleTime: Infinity,
      retry: false,
      meta: { silent: true },
    }),

  tokens: () =>
    queryOptions({
      queryKey: keys.tokens(),
      queryFn: async () => (await unwrap(api.GET('/api/auth/tokens'))).items ?? [],
    }),
}

// --- Mutations ---

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: LoginInput) => unwrap(api.POST('/api/auth/login', { body })),
    // S 2FA přijde jen `two_factor` (bez session) — přihlášení dokončí druhý krok.
    onSuccess: (result) => {
      if (result.me) qc.setQueryData(keys.me(), result.me)
    },
    meta: { silent: true },
  })
}

/** Druhý krok přihlášení kódem z aplikace nebo záložním kódem. */
export function useLoginCode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { token: string; code: string }) => unwrap(api.POST('/api/auth/login/2fa', { body })),
    onSuccess: (me) => qc.setQueryData(keys.me(), me),
    meta: { silent: true },
  })
}

/** Druhý krok přihlášení bezpečnostním klíčem (volby → prohlížeč → ověření). */
export function useLoginWebAuthn() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (token: string) => {
      const { options } = await unwrap(api.POST('/api/auth/login/webauthn/options', { body: { token } }))
      const credential = await getCredential(options)
      return unwrap(api.POST('/api/auth/login/webauthn', { body: { token, credential } }))
    },
    onSuccess: (me) => qc.setQueryData(keys.me(), me),
    meta: { silent: true },
  })
}

// --- Obnova hesla (veřejné) ---

export function useRequestPasswordReset() {
  return useMutation({
    mutationFn: (email: string) => unwrap(api.POST('/api/auth/password-reset', { body: { email } })),
    meta: { silent: true },
  })
}

export function useConfirmPasswordReset(token: string) {
  return useMutation({
    mutationFn: (password: string) =>
      unwrap(api.POST('/api/auth/password-reset/{token}', { params: { path: { token } }, body: { password } })),
    meta: { silent: true },
  })
}

// --- Ověření e-mailu (SPEC §3.2) ---

/** Pošle přihlášenému uživateli odkaz pro ověření e-mailu. */
export function useRequestEmailVerification() {
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/auth/me/verify-email')),
  })
}

/** Potvrdí ověření e-mailu tokenem z odkazu. */
export function useConfirmEmailVerification() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (token: string) => unwrap(api.POST('/api/auth/verify-email', { body: { token } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.me() }),
    meta: { silent: true },
  })
}

// --- Dvoufázové ověření (nastavení) ---

function useInvalidateTwoFactor() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: keys.twoFactor() })
}

export function useTotpSetup() {
  return useMutation({
    mutationFn: (password: string) => unwrap(api.POST('/api/auth/2fa/totp/setup', { body: { password } })),
    meta: { silent: true },
  })
}

export function useTotpEnable() {
  const invalidate = useInvalidateTwoFactor()
  return useMutation({
    mutationFn: (code: string) => unwrap(api.POST('/api/auth/2fa/totp/enable', { body: { code } })),
    onSuccess: invalidate,
    meta: { silent: true },
  })
}

export function useTotpDisable() {
  const invalidate = useInvalidateTwoFactor()
  return useMutation({
    mutationFn: (password: string) => unwrap(api.DELETE('/api/auth/2fa/totp', { body: { password } })),
    onSuccess: invalidate,
    meta: { silent: true },
  })
}

/** Přidání bezpečnostního klíče: heslo → volby → dialog prohlížeče → uložení. */
export function useAddWebAuthnKey() {
  const invalidate = useInvalidateTwoFactor()
  return useMutation({
    mutationFn: async ({ name, password }: { name: string; password: string }) => {
      const { token, options } = await unwrap(api.POST('/api/auth/2fa/webauthn/options', { body: { password } }))
      const credential = await createCredential(options)
      return unwrap(api.POST('/api/auth/2fa/webauthn', { body: { token, name, credential } }))
    },
    onSuccess: invalidate,
    meta: { silent: true },
  })
}

export function useDeleteWebAuthnKey() {
  const invalidate = useInvalidateTwoFactor()
  return useMutation({
    mutationFn: ({ id, password }: { id: number; password: string }) =>
      unwrap(api.DELETE('/api/auth/2fa/webauthn/{id}', { params: { path: { id } }, body: { password } })),
    onSuccess: invalidate,
    meta: { silent: true },
  })
}

export function useRegenerateRecoveryCodes() {
  const invalidate = useInvalidateTwoFactor()
  return useMutation({
    mutationFn: (password: string) => unwrap(api.POST('/api/auth/2fa/recovery-codes', { body: { password } })),
    onSuccess: invalidate,
    meta: { silent: true },
  })
}

export function useRegister() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: RegisterInput) => unwrap(api.POST('/api/auth/register', { body })),
    onSuccess: (me) => {
      qc.setQueryData(keys.me(), me)
      void qc.invalidateQueries({ queryKey: keys.authStatus() })
    },
    meta: { silent: true },
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST('/api/auth/logout')),
    onSettled: () => {
      // I při chybě zahodíme lokální stav — uživatel chtěl pryč.
      qc.clear()
      qc.setQueryData(keys.me(), null)
    },
  })
}

export function useUpdateMe() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateMeInput) => unwrap(api.PATCH('/api/auth/me', { body })),
    onSuccess: (me) => qc.setQueryData(keys.me(), me),
    meta: { silent: true },
  })
}

export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTokenInput) => unwrap(api.POST('/api/auth/tokens', { body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.tokens() }),
    meta: { silent: true },
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      unwrap(api.DELETE('/api/auth/tokens/{id}', { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.tokens() }),
  })
}
