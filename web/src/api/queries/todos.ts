import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query'
import { api, unwrap } from '../client'
import type { CreateTodoInput, Todo, TodoFilters, TodoList, UpdateTodoInput } from '../types'
import { keys } from './keys'

export const TODOS_PER_PAGE = 30

export const todoQueries = {
  /** Úkoly („Načíst další“). Otevřené řazené podle termínu, hotové podle dokončení. */
  list: (slug: string, filters: TodoFilters, perPage = TODOS_PER_PAGE) =>
    infiniteQueryOptions({
      queryKey: keys.todoList(slug, { ...filters, perPage }),
      queryFn: ({ pageParam, signal }) =>
        unwrap(
          api.GET('/api/accounts/{slug}/todos', {
            params: { path: { slug }, query: { ...filters, page: pageParam, per_page: perPage } },
            signal,
          }),
        ),
      initialPageParam: 1,
      getNextPageParam: (last) => (last.page * last.per_page < last.total ? last.page + 1 : undefined),
      placeholderData: keepPreviousData,
    }),

  /** Počet otevřených úkolů (odznak v navigaci). */
  openCount: (slug: string) =>
    queryOptions({
      queryKey: keys.todoCount(slug),
      queryFn: async ({ signal }) =>
        (
          await unwrap(
            api.GET('/api/accounts/{slug}/todos', {
              params: { path: { slug }, query: { completed: 'false', page: 1, per_page: 1 } },
              signal,
            }),
          )
        ).total,
      // Automatické úkoly vznikají i na pozadí (plánovač) → občas obnovit.
      refetchInterval: 5 * 60_000,
    }),
}

type TodoPages = InfiniteData<TodoList, number>

/** Přepíše úkol ve všech načtených seznamech (optimistická aktualizace). */
function patchCachedTodo(qc: ReturnType<typeof useQueryClient>, slug: string, id: number, patch: (t: Todo) => Todo) {
  qc.setQueriesData<TodoPages>({ queryKey: [...keys.todos(slug), 'list'] }, (data) =>
    data
      ? { ...data, pages: data.pages.map((p) => ({ ...p, items: p.items.map((t) => (t.id === id ? patch(t) : t)) })) }
      : data,
  )
}

export function useToggleTodo(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (todo: Pick<Todo, 'id' | 'completed'>) =>
      unwrap(api.POST('/api/accounts/{slug}/todos/{id}/toggle', { params: { path: { slug, id: todo.id } } })),
    onMutate: async (todo: Pick<Todo, 'id' | 'completed'>) => {
      await qc.cancelQueries({ queryKey: keys.todos(slug) })
      const snapshot = qc.getQueriesData<TodoPages>({ queryKey: [...keys.todos(slug), 'list'] })
      const prevCount = qc.getQueryData<number>(keys.todoCount(slug))
      patchCachedTodo(qc, slug, todo.id, (t) => ({
        ...t,
        completed: !t.completed,
        completed_at: t.completed ? undefined : new Date().toISOString(),
      }))
      if (prevCount !== undefined) qc.setQueryData(keys.todoCount(slug), Math.max(0, prevCount + (todo.completed ? 1 : -1)))
      return { snapshot, prevCount }
    },
    onError: (_err, _todo, ctx) => {
      ctx?.snapshot.forEach(([key, data]) => qc.setQueryData(key, data))
      if (ctx?.prevCount !== undefined) qc.setQueryData(keys.todoCount(slug), ctx.prevCount)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: keys.todos(slug) }),
  })
}

export function useCreateTodo(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTodoInput) => unwrap(api.POST('/api/accounts/{slug}/todos', { params: { path: { slug } }, body })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.todos(slug) }),
    meta: { silent: true },
  })
}

export function useUpdateTodo(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateTodoInput }) =>
      unwrap(api.PATCH('/api/accounts/{slug}/todos/{id}', { params: { path: { slug, id } }, body })),
    onSuccess: (todo) => {
      patchCachedTodo(qc, slug, todo.id, () => todo)
      void qc.invalidateQueries({ queryKey: keys.todos(slug) })
    },
    meta: { silent: true },
  })
}

export function useDeleteTodo(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (todo: Pick<Todo, 'id'>) =>
      unwrap(api.DELETE('/api/accounts/{slug}/todos/{id}', { params: { path: { slug, id: todo.id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.todos(slug) }),
  })
}
