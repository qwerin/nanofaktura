import { useInfiniteQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ListTodoIcon } from 'lucide-react'
import { todoQueries } from '@/api/queries/todos'
import { Skeleton } from '@/components/ui/skeleton'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { TodoItem } from './todo-item'

const OPEN = { completed: 'false' } as const

/** Přehled: 5 nejbližších otevřených úkolů. */
export function TodosWidget({ slug }: { slug: string }) {
  const { canEdit } = useCurrentAccount()
  const list = useInfiniteQuery(todoQueries.list(slug, OPEN, 5))
  const first = list.data?.pages[0]
  const items = first?.items

  return (
    <section className="flex flex-col rounded-xl border bg-card">
      <div className="flex items-center justify-between gap-2 px-4 pt-4 pb-2 md:px-5">
        <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight">
          <ListTodoIcon className="size-4 text-primary" aria-hidden="true" />
          Úkoly
          {first && first.total > 0 && <span className="text-sm font-normal text-muted-foreground">{first.total}</span>}
        </h2>
        <Link to="/a/$slug/todos" params={{ slug }} className="text-sm font-medium text-primary underline-offset-4 hover:underline">
          {first && first.total > 0 ? 'Zobrazit vše' : 'Otevřít'}
        </Link>
      </div>
      {list.isError ? (
        <p className="px-4 pt-2 pb-5 text-sm text-destructive md:px-5">Úkoly se nepodařilo načíst.</p>
      ) : !items ? (
        <div className="flex flex-col gap-2 p-4 pt-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="px-4 pt-2 pb-5 text-sm text-muted-foreground md:px-5">Nic nečeká. Vše je vyřešeno.</p>
      ) : (
        <ul className="divide-y pb-1">
          {items.map((t) => (
            <TodoItem key={t.id} slug={slug} todo={t} canEdit={canEdit} compact />
          ))}
        </ul>
      )}
    </section>
  )
}
