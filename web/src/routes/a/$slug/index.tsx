import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/a/$slug/')({
  beforeLoad: ({ params }) => {
    throw redirect({ to: '/a/$slug/dashboard', params, replace: true })
  },
})
