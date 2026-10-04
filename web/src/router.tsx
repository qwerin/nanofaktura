import type { QueryClient } from '@tanstack/react-query'
import { createRouter } from '@tanstack/react-router'
import { PageError, PageNotFound } from '@/components/page-states'
import { PageSkeleton } from '@/components/skeletons'
import { parseSearch, stringifySearch } from '@/lib/search-params'
import { routeTree } from './routeTree.gen'

export interface RouterContext {
  queryClient: QueryClient
}

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    // Data řídí TanStack Query — router loadery jen předplní cache (ensureQueryData).
    defaultPreload: 'intent',
    defaultPreloadStaleTime: 0,
    defaultPendingComponent: PageSkeleton,
    defaultPendingMs: 150,
    defaultErrorComponent: PageError,
    defaultNotFoundComponent: PageNotFound,
    scrollRestoration: true,
    // Parametry OAuth žádosti beze změny (viz lib/search-params).
    parseSearch,
    stringifySearch,
  })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
