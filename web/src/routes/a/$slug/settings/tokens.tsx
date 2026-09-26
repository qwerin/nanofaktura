import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { KeyRoundIcon, PlusIcon, Trash2Icon, TriangleAlertIcon } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { authQueries, useCreateToken, useRevokeToken } from '@/api/queries/auth'
import type { ApiToken, ApiTokenCreated } from '@/api/types'
import { CopyButton } from '@/components/copy-button'
import { EmptyState } from '@/components/empty-state'
import { TextField } from '@/components/form/fields'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { SettingsPage } from '@/components/settings-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/lib/date'

export const Route = createFileRoute('/a/$slug/settings/tokens')({
  loader: ({ context }) => context.queryClient.prefetchQuery(authQueries.tokens()),
  head: () => ({ meta: [{ title: 'API tokeny · NanoFaktura' }] }),
  component: TokensPage,
})

function TokensPage() {
  const tokens = useQuery(authQueries.tokens())
  const [createOpen, setCreateOpen] = useState(false)
  const [revoking, setRevoking] = useState<ApiToken | null>(null)

  const createAction = {
    label: 'Nový token',
    icon: PlusIcon,
    primary: true,
    onClick: () => setCreateOpen(true),
  }

  return (
    <SettingsPage
      title="API tokeny"
      description={
        <>
          Tokeny umožňují přístup k API za vás — pro skripty a integrace. Posílají se v hlavičce{' '}
          <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">Authorization: Bearer nf_…</code>.
          Token má stejná oprávnění jako váš účet.
        </>
      }
      actions={[createAction]}
    >
      <div className="mb-4 hidden justify-end md:flex">
        <Button onClick={() => setCreateOpen(true)}>
          <PlusIcon data-icon="inline-start" />
          Nový token
        </Button>
      </div>

      {tokens.isError ? (
        <PageError error={tokens.error} reset={() => void tokens.refetch()} />
      ) : (
        <ResponsiveList
          items={tokens.data}
          isLoading={tokens.isPending}
          getKey={(t) => t.id}
          empty={
            <EmptyState
              icon={KeyRoundIcon}
              title="Žádné API tokeny"
              description="Vytvořte token pro připojení skriptu nebo jiné aplikace."
              action={
                <Button onClick={() => setCreateOpen(true)}>
                  <PlusIcon data-icon="inline-start" />
                  Nový token
                </Button>
              }
            />
          }
          columns={[
            { id: 'name', header: 'Název', cell: (t) => <span className="font-medium">{t.name}</span> },
            { id: 'prefix', header: 'Token', cell: (t) => <TokenPrefix prefix={t.prefix} /> },
            { id: 'created', header: 'Vytvořen', cell: (t) => formatDateTime(t.created_at) },
            {
              id: 'used',
              header: 'Naposledy použit',
              cell: (t) => (t.last_used_at ? formatDateTime(t.last_used_at) : <span className="text-muted-foreground">Nikdy</span>),
            },
            {
              id: 'actions',
              header: <span className="sr-only">Akce</span>,
              align: 'right',
              cell: (t) => (
                <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => setRevoking(t)}>
                  Zrušit
                </Button>
              ),
            },
          ]}
          renderCard={(t) => (
            <div className="flex items-start gap-3">
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <span className="truncate font-medium">{t.name}</span>
                <TokenPrefix prefix={t.prefix} />
                <span className="text-xs text-muted-foreground">
                  Vytvořen {formatDateTime(t.created_at)}
                  {' · '}
                  {t.last_used_at ? `použit ${formatDateTime(t.last_used_at)}` : 'nikdy nepoužit'}
                </span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Zrušit token ${t.name}`}
                className="-mt-2 -mr-2 text-destructive hover:text-destructive"
                onClick={() => setRevoking(t)}
              >
                <Trash2Icon />
              </Button>
            </div>
          )}
        />
      )}

      <CreateTokenDialog open={createOpen} onOpenChange={setCreateOpen} />
      <RevokeTokenDialog token={revoking} onClose={() => setRevoking(null)} />
    </SettingsPage>
  )
}

function TokenPrefix({ prefix }: { prefix: string }) {
  return <code className="font-mono text-xs text-muted-foreground">{prefix}…</code>
}

const createSchema = z.object({ name: z.string().trim().min(1, 'Pojmenujte token').max(100, 'Nejvýše 100 znaků') })
type CreateValues = z.infer<typeof createSchema>

function CreateTokenDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const create = useCreateToken()
  const [created, setCreated] = useState<ApiTokenCreated | null>(null)
  const form = useForm<CreateValues>({ resolver: zodResolver(createSchema), defaultValues: { name: '' } })

  const handleOpenChange = (next: boolean) => {
    onOpenChange(next)
    if (!next) {
      // Po zavření token zapomeneme — znovu už ho zobrazit nejde.
      setTimeout(() => {
        setCreated(null)
        form.reset()
      }, 300)
    }
  }

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      setCreated(await create.mutateAsync(values))
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  if (created) {
    return (
      <ResponsiveDialog
        open={open}
        onOpenChange={handleOpenChange}
        title="Token vytvořen"
        description={`Token „${created.name}“ je připraven.`}
        footer={<Button onClick={() => handleOpenChange(false)}>Hotovo</Button>}
      >
        <div className="flex flex-col gap-4">
          <Alert>
            <TriangleAlertIcon />
            <AlertTitle>Zkopírujte si ho hned</AlertTitle>
            <AlertDescription>Z bezpečnostních důvodů ho už znovu nezobrazíme.</AlertDescription>
          </Alert>
          <code
            className="block rounded-lg border bg-muted px-3 py-3 font-mono text-sm break-all select-all"
            aria-label="API token"
          >
            {created.token}
          </code>
          <CopyButton value={created.token} label="Zkopírovat token" className="w-full" />
        </div>
      </ResponsiveDialog>
    )
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Nový API token"
      description="Název vám pomůže poznat, kde se token používá."
      footer={
        <>
          <Button type="submit" form="create-token-form" disabled={create.isPending}>
            {create.isPending && <Spinner data-icon="inline-start" />}
            Vytvořit token
          </Button>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id="create-token-form" onSubmit={onSubmit} noValidate>
        <TextField
          control={form.control}
          name="name"
          label="Název"
          placeholder="např. Export do účetnictví"
          autoComplete="off"
          enterKeyHint="done"
        />
      </form>
    </ResponsiveDialog>
  )
}

function RevokeTokenDialog({ token, onClose }: { token: ApiToken | null; onClose: () => void }) {
  const revoke = useRevokeToken()
  return (
    <ResponsiveDialog
      open={token !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Zrušit token?"
      description={
        token && (
          <>
            Aplikace používající token <strong className="text-foreground">{token.name}</strong> ztratí přístup. Akci nelze vrátit.
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() =>
              token &&
              revoke.mutate(token.id, {
                onSuccess: () => {
                  toast.success('Token zrušen')
                  onClose()
                },
              })
            }
          >
            {revoke.isPending && <Spinner data-icon="inline-start" />}
            Zrušit token
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
