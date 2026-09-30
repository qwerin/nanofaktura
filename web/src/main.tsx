import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { setUnauthorizedHandler } from '@/api/client'
import { keys } from '@/api/queries/keys'
import { createQueryClient } from '@/api/query-client'
import { KeyboardInsetsWatcher } from '@/components/keyboard-insets'
import { ThemeProvider } from '@/components/theme'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { PUBLIC_AUTH_PAGES } from '@/lib/safe-redirect'
import { createAppRouter } from './router'
import './index.css'

const queryClient = createQueryClient()
const router = createAppRouter(queryClient)

// Vypršelá session kdekoli v aplikaci → zahodit data a na přihlášení (s návratem zpět).
setUnauthorizedHandler(() => {
  const { pathname, href } = router.state.location
  if (PUBLIC_AUTH_PAGES.some((p) => pathname === p || pathname.startsWith(`${p}/`))) return
  queryClient.clear()
  queryClient.setQueryData(keys.me(), null)
  void router.navigate({ to: '/login', search: { redirect: href }, replace: true })
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <RouterProvider router={router} />
          <Toaster position="top-center" richColors closeButton />
          <KeyboardInsetsWatcher />
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
