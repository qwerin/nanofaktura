import { useQuery } from '@tanstack/react-query'
import { EllipsisVerticalIcon, MailIcon, MailCheckIcon, SearchIcon, ShieldOffIcon, UsersIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { adminQueries, useAdminUserAction, type AdminUserAction } from '@/api/queries/admin'
import type { AdminUser } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { PageError } from '@/components/page-states'
import { ListSkeleton } from '@/components/skeletons'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { formatDate } from '@/lib/date'

const actionTexts: Record<AdminUserAction, { done: string }> = {
  'reset-2fa': { done: 'Dvoufázové ověření je vypnuté.' },
  'verify-email': { done: 'E-mail je označen jako ověřený.' },
  'send-verification': { done: 'Ověřovací odkaz odeslán.' },
}

export function UsersPanel() {
  const [query, setQuery] = useState('')
  const users = useQuery(adminQueries.users(query.trim()))
  const action = useAdminUserAction()
  const [confirmReset, setConfirmReset] = useState<AdminUser | null>(null)

  const run = (u: AdminUser, a: AdminUserAction) =>
    action.mutate({ id: u.id, action: a }, { onSuccess: () => toast.success(actionTexts[a].done) })

  return (
    <div className="flex flex-col gap-4">
      <div className="relative max-w-md">
        <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Hledat podle e-mailu nebo jména"
          aria-label="Hledat uživatele"
          className="h-11 pl-9 md:h-9"
        />
      </div>

      {users.isPending ? (
        <ListSkeleton rows={4} />
      ) : users.isError ? (
        <PageError error={users.error} reset={() => void users.refetch()} />
      ) : (users.data.items ?? []).length === 0 ? (
        <EmptyState icon={UsersIcon} title="Nikdo nenalezen" />
      ) : (
        <Card className="gap-0 py-0">
          <p className="border-b px-4 py-2 text-xs text-muted-foreground">Celkem {users.data.total}</p>
          <ul className="divide-y">
            {(users.data.items ?? []).map((u) => (
              <li key={u.id} className="flex items-start gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{u.name || u.email}</p>
                  <p className="truncate text-sm text-muted-foreground">{u.email}</p>
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {u.instance_admin && <Badge>Správce instance</Badge>}
                    {u.admin_listed && !u.instance_admin && <Badge variant="outline">Správce po ověření e-mailu</Badge>}
                    {u.email_verified_at ? (
                      <Badge variant="secondary">E-mail ověřen</Badge>
                    ) : (
                      <Badge variant="outline">E-mail neověřen</Badge>
                    )}
                    {u.two_factor && <Badge variant="secondary">2FA</Badge>}
                    <Badge variant="outline">
                      {u.accounts} {u.accounts === 1 ? 'účet' : u.accounts >= 2 && u.accounts <= 4 ? 'účty' : 'účtů'}
                    </Badge>
                    <span className="text-xs text-muted-foreground">od {formatDate(u.created_at.slice(0, 10))}</span>
                  </div>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={<Button variant="ghost" size="icon" aria-label={`Akce pro ${u.email}`} className="size-11 md:size-8" />}
                  >
                    <EllipsisVerticalIcon />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="min-w-56">
                    {!u.email_verified_at && (
                      <>
                        <DropdownMenuItem onClick={() => run(u, 'send-verification')}>
                          <MailIcon />
                          Poslat ověřovací odkaz
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => run(u, 'verify-email')}>
                          <MailCheckIcon />
                          Označit e-mail jako ověřený
                        </DropdownMenuItem>
                      </>
                    )}
                    <DropdownMenuItem disabled={!u.two_factor} onClick={() => setConfirmReset(u)} variant="destructive">
                      <ShieldOffIcon />
                      Vypnout dvoufázové ověření
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <AlertDialog open={confirmReset !== null} onOpenChange={(o) => !o && setConfirmReset(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Vypnout dvoufázové ověření?</AlertDialogTitle>
            <AlertDialogDescription>
              {confirmReset?.email} se pak přihlásí jen heslem a ověření si může znovu zapnout. Udělejte to jen tehdy, když jste
              si jistí, že o to žádá skutečně majitel účtu.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Zrušit</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (confirmReset) run(confirmReset, 'reset-2fa')
                setConfirmReset(null)
              }}
            >
              Vypnout
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
