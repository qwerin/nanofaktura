import { keepPreviousData, useInfiniteQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { ChevronRightIcon, SearchIcon, SearchXIcon, UserPlusIcon, UsersIcon, XIcon } from 'lucide-react'
import { useEffect, useEffectEvent, useState, type ReactNode } from 'react'
import { z } from 'zod'
import { subjectQueries } from '@/api/queries/subjects'
import type { Subject } from '@/api/types'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { useExportMenu } from '@/components/export/use-export-menu'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ResponsiveList } from '@/components/responsive-list'
import { subjectTypeLabels } from '@/components/subject/labels'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatZip } from '@/lib/contact'

const searchSchema = z.object({
  query: z.string().optional().catch(undefined),
  type: z.enum(['customer', 'supplier', 'both']).optional().catch(undefined),
})

export const Route = createFileRoute('/a/$slug/subjects/')({
  validateSearch: searchSchema,
  loaderDeps: ({ search }) => ({ query: search.query, type: search.type }),
  loader: ({ context, params, deps }) =>
    context.queryClient.prefetchInfiniteQuery(subjectQueries.list(params.slug, deps)),
  head: () => ({ meta: [{ title: 'Kontakty · NanoFaktura' }] }),
  component: SubjectsPage,
})

const typeFilters = [
  { value: 'all', label: 'Vše' },
  { value: 'customer', label: 'Odběratelé' },
  { value: 'supplier', label: 'Dodavatelé' },
] as const

function pluralContacts(n: number): string {
  if (n === 1) return '1 kontakt'
  if (n >= 2 && n <= 4) return `${n} kontakty`
  return `${n.toLocaleString('cs-CZ')} kontaktů`
}

function SubjectsPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const { canEdit } = useCurrentAccount()
  const navigate = useNavigate({ from: Route.fullPath })
  const filters = { query: search.query, type: search.type }
  const list = useInfiniteQuery({ ...subjectQueries.list(slug, filters), placeholderData: keepPreviousData })
  const exportMenu = useExportMenu({ slug, kind: 'subjects', filters })

  const items = list.data?.pages.flatMap((p) => p.items ?? [])
  const total = list.data?.pages[0]?.total ?? 0
  const filtered = Boolean(search.query || search.type)

  const openSubject = (s: Subject) =>
    navigate({ to: '/a/$slug/subjects/$subjectId', params: { slug, subjectId: s.id } })

  return (
    <>
      <PageHeader
        title="Kontakty"
        description="Odběratelé a dodavatelé, kterým vystavujete doklady."
        actions={
          canEdit
            ? [
                {
                  label: 'Nový kontakt',
                  icon: UserPlusIcon,
                  primary: true,
                  render: <Link to="/a/$slug/subjects/new" params={{ slug }} />,
                },
                exportMenu.action,
              ]
            : [exportMenu.action]
        }
      >
        <div className="flex flex-col gap-2 pt-1 sm:flex-row sm:items-center">
          <SearchInput
            value={search.query ?? ''}
            onChange={(query) =>
              void navigate({ search: (prev) => ({ ...prev, query: query || undefined }), replace: true })
            }
          />
          <Tabs
            value={search.type ?? 'all'}
            onValueChange={(v) =>
              void navigate({
                search: (prev) => ({ ...prev, type: v === 'all' ? undefined : (v as 'customer' | 'supplier') }),
                replace: true,
              })
            }
          >
            <TabsList className="h-11! w-full sm:w-auto md:h-8!">
              {typeFilters.map((f) => (
                <TabsTrigger key={f.value} value={f.value} className="px-3">
                  {f.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
      </PageHeader>

      <PageBody>
        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : (
          <>
            {items && items.length > 0 && (
              <p className="mb-3 flex items-center gap-2 text-sm text-muted-foreground" aria-live="polite">
                {pluralContacts(total)}
                {list.isFetching && !list.isFetchingNextPage && <Spinner className="size-3.5" />}
              </p>
            )}
            <ResponsiveList
              items={items}
              isLoading={list.isPending}
              getKey={(s) => s.id}
              onRowClick={openSubject}
              empty={
                filtered ? (
                  <EmptyState
                    icon={SearchXIcon}
                    title="Nic nenalezeno"
                    description="Zkuste jiný výraz — hledá se v názvu, IČO a e-mailu."
                    action={
                      <Button variant="outline" onClick={() => void navigate({ search: {}, replace: true })}>
                        Zrušit filtry
                      </Button>
                    }
                  />
                ) : (
                  <EmptyState
                    icon={UsersIcon}
                    title="Zatím žádné kontakty"
                    description="Přidejte odběratele — stačí IČO, zbytek doplníme z ARES."
                    action={
                      canEdit && <ButtonLink to="/a/$slug/subjects/new" params={{ slug }}>
                        <UserPlusIcon data-icon="inline-start" />
                        Nový kontakt
                      </ButtonLink>
                    }
                  />
                )
              }
              columns={[
                {
                  id: 'name',
                  header: 'Název',
                  cell: (s) => (
                    <div className="flex min-w-0 flex-col">
                      <span className="truncate font-medium">{s.name}</span>
                      {s.full_name && <span className="truncate text-xs text-muted-foreground">{s.full_name}</span>}
                    </div>
                  ),
                  className: 'max-w-72',
                },
                {
                  id: 'ico',
                  header: 'IČO',
                  cell: (s) => <span className="font-mono text-sm tabular-nums">{s.registration_no || '—'}</span>,
                },
                { id: 'city', header: 'Město', cell: (s) => s.city || <Muted>—</Muted> },
                {
                  id: 'email',
                  header: 'E-mail',
                  cell: (s) => (s.email ? <span className="block max-w-56 truncate">{s.email}</span> : <Muted>—</Muted>),
                },
                {
                  id: 'type',
                  header: 'Typ',
                  cell: (s) => <TypeBadge subject={s} />,
                },
              ]}
              renderCard={(s) => (
                <div className="flex items-center gap-3">
                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">{s.name}</span>
                      {s.type !== 'customer' && <TypeBadge subject={s} />}
                    </div>
                    <span className="truncate text-sm text-muted-foreground">
                      {[s.registration_no && `IČO ${s.registration_no}`, s.city].filter(Boolean).join(' · ') ||
                        (s.zip ? formatZip(s.zip, s.country) : 'Bez IČO a adresy')}
                    </span>
                    {s.email && <span className="truncate text-sm text-muted-foreground">{s.email}</span>}
                  </div>
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                </div>
              )}
              footer={
                list.hasNextPage && (
                  <Button
                    variant="outline"
                    className="w-full sm:w-auto"
                    onClick={() => void list.fetchNextPage()}
                    disabled={list.isFetchingNextPage}
                  >
                    {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
                    Načíst další
                  </Button>
                )
              }
            />
          </>
        )}
      </PageBody>
      {exportMenu.drawer}
    </>
  )
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>
}

function TypeBadge({ subject }: { subject: Pick<Subject, 'type'> }) {
  return (
    <Badge variant={subject.type === 'customer' ? 'outline' : 'secondary'} className="shrink-0">
      {subject.type === 'both' ? 'Oboje' : subjectTypeLabels[subject.type]}
    </Badge>
  )
}

/** Hledání s prodlevou 300 ms; hodnota žije v URL (`?query=`). */
function SearchInput({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const [draft, setDraft] = useState(value)
  const [synced, setSynced] = useState(value)

  // URL se změnila zvenku (zpět v historii, „Zrušit filtry“) → převzít do pole.
  if (value !== synced) {
    setSynced(value)
    setDraft(value)
  }

  const commit = useEffectEvent((next: string) => {
    setSynced(next)
    onChange(next)
  })

  useEffect(() => {
    const next = draft.trim()
    if (next === value) return
    const t = setTimeout(() => commit(next), 300)
    return () => clearTimeout(t)
  }, [draft, value])

  return (
    <InputGroup className="sm:max-w-sm">
      <InputGroupAddon>
        <SearchIcon />
      </InputGroupAddon>
      <InputGroupInput
        type="search"
        inputMode="search"
        enterKeyHint="search"
        placeholder="Hledat název, IČO, e-mail…"
        aria-label="Hledat kontakty"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      />
      {draft && (
        <InputGroupAddon align="inline-end">
          <InputGroupButton size="icon-sm" aria-label="Vymazat hledání" onClick={() => setDraft('')}>
            <XIcon />
          </InputGroupButton>
        </InputGroupAddon>
      )}
    </InputGroup>
  )
}
