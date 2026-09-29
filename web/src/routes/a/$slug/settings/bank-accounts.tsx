import { useIsMutating, useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { LandmarkIcon, LockIcon, PencilIcon, PlusIcon, RefreshCwIcon, StarIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { accountQueries } from '@/api/queries/accounts'
import { keys } from '@/api/queries/keys'
import { bankAccountQueries, useDeleteBankAccount, useUpdateBankAccount } from '@/api/queries/bank-accounts'
import type { BankAccount } from '@/api/types'
import { BankAccountForm } from '@/components/bank-account/bank-account-form'
import { EmptyState } from '@/components/empty-state'
import { PageError } from '@/components/page-states'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { SettingsPage } from '@/components/settings-page'
import { ListSkeleton } from '@/components/skeletons'
import { CopyIconButton } from '@/components/subject/copy-icon-button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { bankName, formatIban } from '@/lib/bank'
import { formatDateTime } from '@/lib/date'

export const Route = createFileRoute('/a/$slug/settings/bank-accounts')({
  loader: ({ context, params }) =>
    Promise.all([
      context.queryClient.prefetchQuery(bankAccountQueries.list(params.slug)),
      context.queryClient.prefetchQuery(accountQueries.detail(params.slug)),
    ]),
  head: () => ({ meta: [{ title: 'Bankovní účty · NanoFaktura' }] }),
  component: BankAccountsPage,
})

type DialogState = { mode: 'create' } | { mode: 'edit'; account: BankAccount } | null

function BankAccountsPage() {
  const { slug } = Route.useParams()
  const { canManageSettings } = useCurrentAccount()
  const list = useQuery(bankAccountQueries.list(slug))
  const account = useQuery(accountQueries.detail(slug))
  const [dialog, setDialog] = useState<DialogState>(null)
  const [deleting, setDeleting] = useState<BankAccount | null>(null)
  const setDefault = useUpdateBankAccount(slug)

  const openCreate = () => setDialog({ mode: 'create' })

  return (
    <SettingsPage
      title="Bankovní účty"
      description="Účty, na které vám odběratelé platí. Výchozí účet pro každou měnu se předvyplní na fakturu a do QR platby."
      actions={canManageSettings ? [{ label: 'Přidat účet', icon: PlusIcon, primary: true, onClick: openCreate }] : undefined}
    >
      {!canManageSettings && (
        <Alert className="mb-6 max-w-2xl">
          <LockIcon />
          <AlertDescription>Bankovní účty může měnit jen vlastník nebo administrátor účtu.</AlertDescription>
        </Alert>
      )}

      {canManageSettings && list.data && list.data.length > 0 && (
        <div className="mb-4 hidden justify-end md:flex">
          <Button onClick={openCreate}>
            <PlusIcon data-icon="inline-start" />
            Přidat účet
          </Button>
        </div>
      )}

      {list.isError ? (
        <PageError error={list.error} reset={() => void list.refetch()} />
      ) : list.isPending ? (
        <ListSkeleton rows={2} />
      ) : list.data.length === 0 ? (
        <EmptyState
          icon={LandmarkIcon}
          title="Zatím žádný bankovní účet"
          description="Přidejte účet, na který chcete dostávat platby. Na fakturách se pak zobrazí i QR platba."
          action={
            canManageSettings && (
              <Button onClick={openCreate}>
                <PlusIcon data-icon="inline-start" />
                Přidat účet
              </Button>
            )
          }
        />
      ) : (
        <ul className="grid gap-3 lg:grid-cols-2">
          {list.data.map((a) => (
            <li key={a.id}>
              <BankAccountCard
                account={a}
                canManage={canManageSettings}
                onEdit={() => setDialog({ mode: 'edit', account: a })}
                onDelete={() => setDeleting(a)}
                onSetDefault={() =>
                  setDefault.mutate(
                    { id: a.id, body: { is_default: true } },
                    { onSuccess: () => toast.success(`„${a.name}“ je nyní výchozí pro ${a.currency}`) },
                  )
                }
                settingDefault={setDefault.isPending && setDefault.variables?.id === a.id}
              />
            </li>
          ))}
        </ul>
      )}

      <BankAccountDialog
        slug={slug}
        state={dialog}
        onClose={() => setDialog(null)}
        defaultCurrency={account.data?.default_currency || 'CZK'}
        isFirst={list.data?.length === 0}
      />
      <DeleteBankAccountDialog
        slug={slug}
        account={deleting}
        onClose={() => setDeleting(null)}
        othersInCurrency={deleting ? (list.data ?? []).filter((a) => a.currency === deleting.currency && a.id !== deleting.id).length : 0}
      />
    </SettingsPage>
  )
}

function BankAccountCard({
  account: a,
  canManage,
  onEdit,
  onDelete,
  onSetDefault,
  settingDefault,
}: {
  account: BankAccount
  canManage: boolean
  onEdit: () => void
  onDelete: () => void
  onSetDefault: () => void
  settingDefault: boolean
}) {
  const bankCode = a.number.split('/')[1]
  const bank = bankCode ? bankName(bankCode) : undefined

  return (
    <div className="flex h-full flex-col rounded-xl border bg-card text-card-foreground">
      <div className="flex items-start gap-3 p-4">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <LandmarkIcon className="size-5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h3 className="truncate font-medium">{a.name}</h3>
            {a.is_default && (
              <Badge className="border-transparent bg-success/10 text-success">
                <StarIcon data-icon="inline-start" />
                Výchozí
              </Badge>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            {[bank ?? (a.number ? `Banka ${bankCode}` : 'Zahraniční účet'), a.currency].join(' · ')}
          </p>
          {a.sync_provider === 'fio' && (
            <p className="mt-1 flex items-center gap-1.5 text-xs text-info">
              <RefreshCwIcon className="size-3.5 shrink-0" />
              {a.last_synced_at ? `Fio synchronizace · naposledy ${formatDateTime(a.last_synced_at)}` : 'Fio synchronizace zapnutá'}
            </p>
          )}
        </div>
      </div>

      <dl className="flex flex-col border-t text-sm">
        {a.number && <CopyRow label="Účet" value={a.number} emphasize />}
        {a.iban && <CopyRow label="IBAN" value={a.iban} display={formatIban(a.iban)} emphasize={!a.number} />}
        {a.swift_bic && <CopyRow label="SWIFT" value={a.swift_bic} />}
      </dl>

      {canManage && (
        <div className="mt-auto flex items-center gap-1 border-t p-2">
          <Button variant="ghost" onClick={onEdit}>
            <PencilIcon data-icon="inline-start" />
            Upravit
          </Button>
          {!a.is_default && (
            <Button variant="ghost" onClick={onSetDefault} disabled={settingDefault}>
              {settingDefault ? <Spinner data-icon="inline-start" /> : <StarIcon data-icon="inline-start" />}
              Nastavit výchozí
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon"
            className="ml-auto text-destructive hover:text-destructive"
            aria-label={`Smazat účet ${a.name}`}
            onClick={onDelete}
          >
            <Trash2Icon />
          </Button>
        </div>
      )}
    </div>
  )
}

function CopyRow({ label, value, display, emphasize }: { label: string; value: string; display?: string; emphasize?: boolean }) {
  return (
    <div className="flex min-h-11 items-center gap-3 border-b px-4 py-1 last:border-b-0">
      <dt className="w-16 shrink-0 text-muted-foreground sm:w-20">{label}</dt>
      <dd className={emphasize ? 'min-w-0 flex-1 font-mono font-medium wrap-anywhere' : 'min-w-0 flex-1 font-mono wrap-anywhere'}>
        {display ?? value}
      </dd>
      <CopyIconButton value={value} label={`Kopírovat ${label}`} className="-mr-2" />
    </div>
  )
}

function BankAccountDialog({
  slug,
  state,
  onClose,
  defaultCurrency,
  isFirst,
}: {
  slug: string
  state: DialogState
  onClose: () => void
  defaultCurrency: string
  isFirst: boolean
}) {
  const editing = state?.mode === 'edit' ? state.account : undefined
  const saving = useIsMutating({ mutationKey: [...keys.bankAccounts(slug), 'save'] }) > 0
  return (
    <ResponsiveDialog
      open={state !== null}
      onOpenChange={(o) => !o && onClose()}
      title={editing ? 'Upravit bankovní účet' : 'Nový bankovní účet'}
      description={editing ? editing.name : 'Český účet stačí zadat číslem, IBAN dopočítáme.'}
      footer={
        <>
          <Button type="submit" form="bank-account-form" disabled={saving}>
            {saving && <Spinner data-icon="inline-start" />}
            {editing ? 'Uložit' : 'Přidat účet'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            Zrušit
          </Button>
        </>
      }
    >
      {state && (
        <BankAccountForm
          key={editing?.id ?? 'new'}
          slug={slug}
          account={editing}
          defaultCurrency={defaultCurrency}
          isFirst={isFirst}
          formId="bank-account-form"
          onSaved={onClose}
        />
      )}
    </ResponsiveDialog>
  )
}

function DeleteBankAccountDialog({
  slug,
  account,
  onClose,
  othersInCurrency,
}: {
  slug: string
  account: BankAccount | null
  onClose: () => void
  othersInCurrency: number
}) {
  const del = useDeleteBankAccount(slug)
  return (
    <ResponsiveDialog
      open={account !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Smazat bankovní účet?"
      description={
        account && (
          <>
            Účet <strong className="text-foreground">{account.name}</strong> se už nebude nabízet na nových fakturách.
            Vystavené faktury si číslo účtu ponechají.
            {account.is_default && othersInCurrency > 0 && ` Výchozím pro ${account.currency} se stane jiný účet.`}
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={() =>
              account &&
              del.mutate(account, {
                onSuccess: () => {
                  toast.success('Bankovní účet smazán')
                  onClose()
                },
              })
            }
          >
            {del.isPending && <Spinner data-icon="inline-start" />}
            Smazat účet
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
