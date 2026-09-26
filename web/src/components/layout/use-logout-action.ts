import { useNavigate } from '@tanstack/react-router'
import { useLogout } from '@/api/queries/auth'

/** Odhlášení + přesměrování na /login (i když server neodpoví). */
export function useLogoutAction() {
  const logout = useLogout()
  const navigate = useNavigate()
  return {
    isPending: logout.isPending,
    run: () =>
      logout.mutate(undefined, {
        onSettled: () => void navigate({ to: '/login', replace: true }),
      }),
  }
}
