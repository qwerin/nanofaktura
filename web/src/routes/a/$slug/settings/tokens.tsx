import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { BotIcon, KeyRoundIcon, PlugIcon, PlusIcon, Trash2Icon, TriangleAlertIcon, UnplugIcon } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { authQueries, useCreateToken, useRevokeOAuthGrant, useRevokeToken } from '@/api/queries/auth'
import type { ApiToken, ApiTokenCreated, OAuthGrant } from '@/api/types'
import { CopyButton } from '@/components/copy-button'
import { EmptyState } from '@/components/empty-state'
import { SelectField, TextField } from '@/components/form/fields'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ResponsiveList } from '@/components/responsive-list'
import { SettingsPage } from '@/components/settings-page'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { formatDateTime } from '@/lib/date'

export const Route = createFileRoute('/a/$slug/settings/tokens')({
  loader: ({ context }) =>
    Promise.all([
      context.queryClient.prefetchQuery(authQueries.tokens()),
      context.queryClient.prefetchQuery(authQueries.oauthGrants()),
    ]),
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
              id: 'expires',
              header: 'Platí do',
              cell: (t) => (t.expires_at ? formatDateTime(t.expires_at) : <span className="text-muted-foreground">Bez omezení</span>),
            },
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
                  {t.expires_at && ` · platí do ${formatDateTime(t.expires_at)}`}
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

      <McpCard />
      <ConnectedApps />

      <CreateTokenDialog open={createOpen} onOpenChange={setCreateOpen} />
      <RevokeTokenDialog token={revoking} onClose={() => setRevoking(null)} />
    </SettingsPage>
  )
}

function TokenPrefix({ prefix }: { prefix: string }) {
  return <code className="font-mono text-xs text-muted-foreground">{prefix}…</code>
}

/** Adresa MCP serveru této instance (stejný původ jako aplikace). */
function mcpUrl() {
  return `${window.location.origin}/api/mcp`
}

/** Příkaz pro připojení AI asistenta (Claude Code) s daným tokenem. */
function mcpCommand(token: string) {
  return `claude mcp add --transport http nanofaktura ${mcpUrl()} --header "Authorization: Bearer ${token}"`
}

function McpCard() {
  return (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BotIcon className="size-4" />
          AI asistent (MCP)
        </CardTitle>
        <CardDescription>
          Připojte AI asistenta (např. Claude) přes protokol MCP. Asistent pak umí hledat a číst faktury, náklady a kontakty,
          připravit fakturu, zapsat náklad nebo úhradu. Nic neodesílá e-mailem, nic nemaže a nemění nastavení. Má stejná
          oprávnění jako vy.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <code className="block rounded-lg border bg-muted px-3 py-2 font-mono text-sm break-all select-all">{mcpUrl()}</code>
        <div className="flex flex-col gap-2 sm:flex-row">
          <CopyButton value={mcpUrl()} label="Zkopírovat adresu" />
          <CopyButton value={mcpCommand('nf_…')} label="Příkaz pro Claude Code" />
        </div>
        <p className="text-xs text-muted-foreground">
          <strong className="font-medium text-foreground">claude.ai:</strong> přidejte NanoFakturu jako vlastní konektor s touto
          adresou a přihlaste se — token není potřeba. <strong className="font-medium text-foreground">Claude Code</strong> a
          další klienti se přihlašují API tokenem; po vytvoření nového tokenu nabídneme příkaz i s ním.
        </p>
      </CardContent>
    </Card>
  )
}

const createSchema = z.object({
  name: z.string().trim().min(1, 'Pojmenujte token').max(100, 'Nejvýše 100 znaků'),
  expires: z.enum(['30', '90', '365', 'never']),
})

const expiryOptions = [
  { value: '30', label: '30 dní' },
  { value: '90', label: '90 dní' },
  { value: '365', label: '1 rok' },
  { value: 'never', label: 'Bez omezení' },
] as const
type CreateValues = z.infer<typeof createSchema>

function CreateTokenDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const create = useCreateToken()
  const [created, setCreated] = useState<ApiTokenCreated | null>(null)
  const form = useForm<CreateValues>({ resolver: zodResolver(createSchema), defaultValues: { name: '', expires: '90' } })

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
      const days = values.expires === 'never' ? undefined : Number(values.expires)
      setCreated(await create.mutateAsync({ name: values.name, ...(days ? { expires_in_days: days } : {}) }))
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
          <CopyButton value={mcpCommand(created.token)} label="Příkaz pro AI asistenta (MCP)" className="w-full" />
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
      <form id="create-token-form" onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
        <TextField
          control={form.control}
          name="name"
          label="Název"
          placeholder="např. Export do účetnictví"
          autoComplete="off"
          enterKeyHint="done"
        />
        <SelectField
          control={form.control}
          name="expires"
          label="Platnost"
          options={expiryOptions}
          description="Po uplynutí token přestane fungovat. Všechny tokeny zneplatní i změna hesla."
        />
      </form>
    </ResponsiveDialog>
  )
}

/** Aplikace připojené přes OAuth (např. konektor v claude.ai) — mají přístup za uživatele bez API tokenu. */
function ConnectedApps() {
  const grants = useQuery(authQueries.oauthGrants())
  const [revoking, setRevoking] = useState<OAuthGrant | null>(null)
  const disconnect = (g: OAuthGrant) => (
    <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => setRevoking(g)}>
      Odpojit
    </Button>
  )

  return (
    <section className="mt-8 flex flex-col gap-3" aria-labelledby="connected-apps">
      <div className="flex flex-col gap-1">
        <h2 id="connected-apps" className="font-semibold">
          Připojené aplikace
        </h2>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Aplikace, kterým jste povolili přístup přihlášením (např. Claude). Mají stejná oprávnění jako vy. Po odpojení
          přístup okamžitě ztratí.
        </p>
      </div>
      {grants.isError ? (
        <PageError error={grants.error} reset={() => void grants.refetch()} />
      ) : (
        <ResponsiveList
          items={grants.data}
          isLoading={grants.isPending}
          getKey={(g) => g.id}
          empty={
            <EmptyState
              icon={PlugIcon}
              title="Žádné připojené aplikace"
              description="Až aplikaci povolíte přístup, uvidíte ji tady."
              className="py-8"
            />
          }
          columns={[
            { id: 'name', header: 'Aplikace', cell: (g) => <span className="font-medium">{g.client_name}</span> },
            { id: 'created', header: 'Připojena', cell: (g) => formatDateTime(g.created_at) },
            {
              id: 'used',
              header: 'Naposledy použita',
              cell: (g) => (g.last_used_at ? formatDateTime(g.last_used_at) : <span className="text-muted-foreground">Nikdy</span>),
            },
            { id: 'actions', header: <span className="sr-only">Akce</span>, align: 'right', cell: disconnect },
          ]}
          renderCard={(g) => (
            <div className="flex items-start gap-3">
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <span className="truncate font-medium">{g.client_name}</span>
                <span className="text-xs text-muted-foreground">
                  Připojena {formatDateTime(g.created_at)}
                  {' · '}
                  {g.last_used_at ? `použita ${formatDateTime(g.last_used_at)}` : 'nikdy nepoužita'}
                </span>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Odpojit aplikaci ${g.client_name}`}
                className="-mt-2 -mr-2 text-destructive hover:text-destructive"
                onClick={() => setRevoking(g)}
              >
                <UnplugIcon />
              </Button>
            </div>
          )}
        />
      )}
      <DisconnectAppDialog grant={revoking} onClose={() => setRevoking(null)} />
    </section>
  )
}

function DisconnectAppDialog({ grant, onClose }: { grant: OAuthGrant | null; onClose: () => void }) {
  const revoke = useRevokeOAuthGrant()
  return (
    <ResponsiveDialog
      open={grant !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Odpojit aplikaci?"
      description={
        grant && (
          <>
            Aplikace <strong className="text-foreground">{grant.client_name}</strong> ztratí přístup k vašim účtům. Znovu ji
            můžete připojit, přístup ale budete muset povolit znovu.
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={revoke.isPending}
            onClick={() =>
              grant &&
              revoke.mutate(grant.id, {
                onSuccess: () => {
                  toast.success('Aplikace odpojena')
                  onClose()
                },
              })
            }
          >
            {revoke.isPending && <Spinner data-icon="inline-start" />}
            Odpojit
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
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
