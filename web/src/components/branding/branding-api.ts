// Logo a podpis/razítko účtu (SPEC §7.12, §7.14): obrázek se nahraje jako příloha účtu
// (`owner_type=account`, `owner_id` backend u účtu ignoruje) a jeho id se uloží do
// `logo_attachment_id` / `stamp_attachment_id` přes PATCH účtu (0 = odebrat).
// Nahrávání a URL přebírá z obecných příloh (api/queries/attachments.ts).

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, unwrap } from '@/api/client'
import { attachmentUrl, uploadAttachment } from '@/api/queries/attachments'
import { keys } from '@/api/queries/keys'
import type { Account } from '@/api/types'

export type BrandingKind = 'logo' | 'stamp'

export const brandingField = {
  logo: 'logo_attachment_id',
  stamp: 'stamp_attachment_id',
} as const satisfies Record<BrandingKind, keyof Account>

/** Formáty, které umí vykreslit PDF (backend jiné pro logo/razítko odmítne 422). */
export const BRANDING_ACCEPT = 'image/png,image/jpeg'
export const BRANDING_MAX_BYTES = 5 * 1024 * 1024

/** Kontrola souboru před nahráním; vrací českou chybu, nebo `null`. */
export function validateBrandingFile(file: Pick<File, 'type' | 'size' | 'name'>): string | null {
  const type = file.type.toLowerCase()
  const byName = /\.(png|jpe?g)$/i.test(file.name)
  if (type ? type !== 'image/png' && type !== 'image/jpeg' : !byName) {
    return 'Nahrajte obrázek PNG nebo JPEG.'
  }
  if (file.size > BRANDING_MAX_BYTES) return 'Obrázek je příliš velký (nejvýše 5 MB).'
  if (file.size === 0) return 'Soubor je prázdný.'
  return null
}

/** URL obrázku pro `<img>` (download inline; session cookie jde automaticky). */
export function brandingImageUrl(slug: string, id: number): string {
  return attachmentUrl(slug, id, { inline: true })
}

async function deleteAttachmentQuietly(slug: string, id: number) {
  try {
    await unwrap(api.DELETE('/api/accounts/{slug}/attachments/{id}', { params: { path: { slug, id } } }))
  } catch {
    // Úklid staré přílohy není kritický (smazání mohlo proběhnout jinde).
  }
}

/**
 * Nastaví (`file`) nebo odebere (`file: null`) logo/razítko.
 * Pořadí: nahrát → PATCH účtu → smazat předchozí přílohu, aby PDF nikdy neukazovalo na neexistující obrázek.
 */
export function useSetBrandingImage(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ kind, file, previousId }: { kind: BrandingKind; file: File | null; previousId?: number }) => {
      // owner_id backend u owner_type=account ignoruje (vždy aktuální účet).
      const uploaded = file ? await uploadAttachment(slug, { ownerType: 'account', ownerId: 0, file }) : null
      let account: Account
      try {
        account = await unwrap(
          api.PATCH('/api/accounts/{slug}', {
            params: { path: { slug } },
            body: { [brandingField[kind]]: uploaded?.id ?? 0 },
          }),
        )
      } catch (err) {
        if (uploaded) await deleteAttachmentQuietly(slug, uploaded.id)
        throw err
      }
      if (previousId && previousId !== uploaded?.id) await deleteAttachmentQuietly(slug, previousId)
      return account
    },
    onSuccess: (account) => {
      qc.setQueryData(keys.accountDetail(slug), account)
      // Náhled PDF obsahuje logo/razítko.
      void qc.invalidateQueries({ queryKey: [...keys.account(slug), 'pdf-preview'] })
    },
    meta: { silent: true },
  })
}
