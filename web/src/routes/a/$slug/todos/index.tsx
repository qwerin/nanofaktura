import { useInfiniteQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { CircleCheckBigIcon, ListTodoIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { z } from 'zod'
import { todoQueries } from '@/api/queries/todos'
import type { Todo, TodoFilters } from '@/api/types'
import { EmptyState } from '@/components/empty-state'
import { PageBody, PageHeader } from '@/components/page-header'
import { PageError } from '@/components/page-states'
import { ListSkeleton } from '@/components/skeletons'
import { DeleteTodoDialog, EditTodoDialog, TodoQuickAdd } from '@/components/todos/todo-dialogs'
import { TodoItem } from '@/components/todos/todo-item'
import { todoTypeFilters } from '@/components/todos/todo-meta'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { cn } from '@/lib/utils'

const relatedTypes = ['invoice', 'expense', 'subject', 'price_item', 'bank_transaction', 'recurring'] as const

const searchSchema = z.object({
  tab: z.enum(['open', 'done']).optional().catch(undefined),
  type: z.enum(relatedTypes).optional().catch(undefined),
})
type TodoSearch = z.infer<typeof searchSchema>

function toFilters(search: TodoSearch): TodoFilters {
  return {
    completed: search.tab === 'done' ? 'true' : 'false',
    ...(search.type ? { related_type: search.type } : {}),
  }
}

export const Route = createFileRoute('/a/$slug/todos/')({
  validateSearch: searchSchema,
  loaderDeps: ({ search }) => toFilters(search),
  loader: ({ context, params, deps }) => context.queryClient.prefetchInfiniteQuery(todoQueries.list(params.slug, deps)),
  head: () => ({ meta: [{ title: 'Úkoly · NanoFaktura' }] }),
  component: TodosPage,
})

function TodosPage() {
  const { slug } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const { canEdit } = useCurrentAccount()
  const done = search.tab === 'done'
  const list = useInfiniteQuery(todoQueries.list(slug, toFilters(search)))
  const [editing, setEditing] = useState<Todo | null>(null)
  const [deleting, setDeleting] = useState<Todo | null>(null)

  const items = list.data?.pages.flatMap((p) => p.items)
  const total = list.data?.pages[0]?.total

  return (
    <>
      <PageHeader title="Úkoly" description="Co je potřeba vyřešit — automatické upozornění i vlastní poznámky.">
        <div className="flex flex-col gap-2 pt-1">
          <Tabs
            value={done ? 'done' : 'open'}
            onValueChange={(v) => void navigate({ search: (prev) => ({ ...prev, tab: v === 'done' ? 'done' : undefined }), replace: true })}
          >
            <TabsList className="h-11! w-full sm:w-auto md:h-8!">
              <TabsTrigger value="open" className="px-4">
                Otevřené
              </TabsTrigger>
              <TabsTrigger value="done" className="px-4">
                Hotové
              </TabsTrigger>
            </TabsList>
          </Tabs>
          {/* Filtr podle typu vazby — vodorovně posuvné čipy (na mobilu bez zalomení) */}
          <div role="group" aria-label="Typ úkolu" className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 [scrollbar-width:none] md:mx-0 md:flex-wrap md:px-0">
            <Chip active={!search.type} onClick={() => void navigate({ search: (prev) => ({ ...prev, type: undefined }), replace: true })}>
              Vše
            </Chip>
            {todoTypeFilters.map((f) => (
              <Chip
                key={f.value}
                active={search.type === f.value}
                onClick={() => void navigate({ search: (prev) => ({ ...prev, type: f.value }), replace: true })}
              >
                {f.label}
              </Chip>
            ))}
          </div>
        </div>
      </PageHeader>

      <PageBody>
        <div className="flex max-w-3xl flex-col gap-4">
        {canEdit && !done && <TodoQuickAdd slug={slug} />}

        {list.isError ? (
          <PageError error={list.error} reset={() => void list.refetch()} />
        ) : !items ? (
          <ListSkeleton />
        ) : items.length === 0 ? (
          done ? (
            <EmptyState icon={ListTodoIcon} title="Zatím nic hotového" description="Dokončené úkoly se zobrazí tady." />
          ) : (
            <EmptyState
              icon={CircleCheckBigIcon}
              title="Vše vyřešeno"
              description={
                search.type
                  ? 'V této kategorii nejsou žádné otevřené úkoly.'
                  : 'Faktury po splatnosti, nespárované platby nebo nízký sklad se tu objeví automaticky.'
              }
            />
          )
        ) : (
          <>
            {total !== undefined && (
              <p className="text-sm text-muted-foreground">
                {total} {done ? 'hotových' : total === 1 ? 'otevřený úkol' : total >= 2 && total <= 4 ? 'otevřené úkoly' : 'otevřených úkolů'}
              </p>
            )}
            <ul className={cn('divide-y rounded-xl border bg-card transition-opacity', list.isPlaceholderData && 'opacity-60')}>
              {items.map((t) => (
                <TodoItem key={t.id} slug={slug} todo={t} canEdit={canEdit} onEdit={setEditing} onDelete={setDeleting} />
              ))}
            </ul>
            {list.hasNextPage && (
              <Button variant="outline" className="self-center" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
                {list.isFetchingNextPage && <Spinner data-icon="inline-start" />}
                Načíst další
              </Button>
            )}
          </>
        )}
        </div>
      </PageBody>

      <EditTodoDialog slug={slug} todo={editing} onClose={() => setEditing(null)} />
      <DeleteTodoDialog slug={slug} todo={deleting} onClose={() => setDeleting(null)} />
    </>
  )
}

function Chip({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'h-11 shrink-0 rounded-full border px-3.5 text-sm font-medium transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none md:h-7 md:px-3 md:text-xs',
        active ? 'border-primary bg-primary text-primary-foreground' : 'bg-background text-muted-foreground hover:text-foreground',
      )}
    >
      {children}
    </button>
  )
}
