import { useCurrentAccount } from '@/hooks/use-current-account'

/** Smí uživatel měnit doklady? (účetní má jen čtení) */
export function useCanEditDocuments(): boolean {
  return useCurrentAccount().canEdit
}
