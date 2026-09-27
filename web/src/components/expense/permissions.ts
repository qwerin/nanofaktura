import { useCurrentAccount } from '@/hooks/use-current-account'
import { canEditRole } from '@/lib/roles'

/** Role owner/admin/member smí měnit doklady, náklady, ceník a přílohy (SPEC §7.13). */
export function canEditDocuments(role: string | undefined): boolean {
  return canEditRole(role as Parameters<typeof canEditRole>[0])
}

/** Smí aktuální uživatel v tomto účtu upravovat doklady (náklady, ceník, přílohy)? */
export function useCanEditDocuments(): boolean {
  return useCurrentAccount().canEdit
}
