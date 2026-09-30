// Render helpery pro komponentové testy.
//
//   renderApp('/a/firma/invoices/new')  — celá aplikace (router, guardy, AppShell) na dané URL
//   renderWithProviders(<Foo />)        — samostatná komponenta s QueryClientem a minimálním routerem
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactElement, ReactNode } from 'react'
import { ThemeProvider } from '@/components/theme'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { createAppRouter } from '@/router'

export function testQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  })
}

function Providers({ client, children }: { client: QueryClient; children: ReactNode }) {
  return (
    <ThemeProvider>
      <QueryClientProvider client={client}>
        <TooltipProvider>
          {children}
          <Toaster />
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  )
}

/** Celá aplikace s paměťovou historií. Vrací i router (router.state.location pro asserce navigace). */
export async function renderApp(url: string) {
  const queryClient = testQueryClient()
  const router = createAppRouter(queryClient)
  router.update({
    ...router.options,
    history: createMemoryHistory({ initialEntries: [url] }),
    defaultPendingMs: 0,
    defaultPreload: false,
  })
  const user = userEvent.setup()
  await router.load()
  const view = render(
    <Providers client={queryClient}>
      <RouterProvider router={router} />
    </Providers>,
  )
  return { ...view, user, router, queryClient }
}

/**
 * Komponenta v minimálním routeru (Link / useParams fungují). `path` je vzor route, `url` skutečná adresa,
 * např. path '/a/$slug' + url '/a/firma' pro komponenty používající useParams({ from: '/a/$slug' }).
 */
export function renderWithProviders(
  ui: ReactElement,
  { path = '/', url = '/', client = testQueryClient() }: { path?: string; url?: string; client?: QueryClient } = {},
) {
  const rootRoute = createRootRoute({ component: Outlet })
  const route = createRoute({ getParentRoute: () => rootRoute, path, component: () => ui })
  const catchAll = createRoute({ getParentRoute: () => rootRoute, path: '$', component: () => <p>jinde</p> })
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, catchAll]),
    history: createMemoryHistory({ initialEntries: [url] }),
  })
  const user = userEvent.setup()
  const view = render(
    <Providers client={client}>
      {/* testovací router mimo registrovaný strom aplikace */}
      <RouterProvider router={router as never} />
    </Providers>,
  )
  return { ...view, user, router, client }
}
