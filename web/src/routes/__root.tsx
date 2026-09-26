import { createRootRouteWithContext, HeadContent, Outlet } from '@tanstack/react-router'
import type { RouterContext } from '@/router'

export const Route = createRootRouteWithContext<RouterContext>()({
  head: () => ({ meta: [{ title: 'NanoFaktura' }] }),
  component: RootLayout,
})

function RootLayout() {
  return (
    <>
      <HeadContent />
      <Outlet />
    </>
  )
}
