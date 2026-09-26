import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { errorMessage, isApiError } from './errors'

declare module '@tanstack/react-query' {
  interface Register {
    queryMeta: {
      /** Nezobrazovat globální toast při chybě (komponenta si ji vyřeší sama). */
      silent?: boolean
    }
    mutationMeta: {
      silent?: boolean
    }
  }
}

function shouldToast(err: unknown, silent: boolean | undefined): boolean {
  if (silent) return false
  // 401 řeší redirect na /login (src/api/client.ts), 404 zobrazí stránka.
  if (isApiError(err) && (err.status === 401 || err.status === 404)) return false
  return true
}

export function createQueryClient(): QueryClient {
  return new QueryClient({
    queryCache: new QueryCache({
      onError(err, query) {
        // Toast jen při chybě obnovy už zobrazených dat; první načtení ukazuje stránka.
        if (query.state.data !== undefined && shouldToast(err, query.meta?.silent)) {
          toast.error(errorMessage(err))
        }
      },
    }),
    mutationCache: new MutationCache({
      onError(err, _vars, _ctx, mutation) {
        if (shouldToast(err, mutation.meta?.silent)) {
          toast.error(errorMessage(err))
        }
      },
    }),
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry(failureCount, err) {
          if (isApiError(err) && err.status >= 400 && err.status < 500) return false
          return failureCount < 2
        },
        refetchOnWindowFocus: true,
      },
      mutations: { retry: false },
    },
  })
}
