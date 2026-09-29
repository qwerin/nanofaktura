import { useQueries } from '@tanstack/react-query'
import { subjectQueries } from '@/api/queries/subjects'
import type { Subject } from '@/api/types'

/**
 * Kontakty podle ID (šablona nese jen `subject_id`). Šablon je málo a detaily kontaktů
 * sdílí cache s jinými stránkami, takže N dotazů je tu v pořádku.
 */
export function useSubjectMap(slug: string, ids: readonly number[]): Map<number, Subject> {
  const unique = [...new Set(ids.filter((id) => id > 0))]
  return useQueries({
    queries: unique.map((id) => ({ ...subjectQueries.detail(slug, id), staleTime: 60_000, retry: false })),
    combine: (results) => {
      const map = new Map<number, Subject>()
      results.forEach((r) => {
        if (r.data) map.set(r.data.id, r.data)
      })
      return map
    },
  })
}
