import { BotIcon, CalendarIcon, ChevronRightIcon, EllipsisVerticalIcon, PencilIcon, Trash2Icon } from 'lucide-react'
import { useToggleTodo } from '@/api/queries/todos'
import type { Todo } from '@/api/types'
import { HrefLink } from '@/components/events/href-link'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { formatDate, formatDateTime, formatDueRelative } from '@/lib/date'
import { recordHref } from '@/lib/record-links'
import { cn } from '@/lib/utils'
import { relatedLinkLabels, todoDueState } from './todo-meta'

const dueClasses = {
  none: 'text-muted-foreground',
  overdue: 'text-destructive font-medium',
  today: 'text-warning-foreground dark:text-warning font-medium',
  soon: 'text-foreground',
  later: 'text-muted-foreground',
} as const

/** Řádek úkolu: zaškrtnutí (optimisticky), text, termín, odkaz na záznam, štítek „auto“, menu úprav. */
export function TodoItem({
  slug,
  todo,
  canEdit,
  compact,
  onEdit,
  onDelete,
}: {
  slug: string
  todo: Todo
  canEdit: boolean
  /** Widget na přehledu: bez menu, menší odsazení. */
  compact?: boolean
  onEdit?: (t: Todo) => void
  onDelete?: (t: Todo) => void
}) {
  const toggle = useToggleTodo(slug)
  const href = recordHref(slug, todo.related_type, todo.related_id)
  const due = todoDueState(todo)
  const editable = canEdit && !todo.automatic && !compact && (onEdit || onDelete)

  return (
    <li className={cn('flex items-start gap-1 py-1', compact ? 'px-2 md:px-3' : 'px-1 md:px-2')}>
      {/* Velká dotyková plocha kolem checkboxu (≥ 44 px) */}
      <label className="flex size-11 shrink-0 cursor-pointer items-center justify-center md:size-9">
        <Checkbox
          checked={todo.completed}
          disabled={!canEdit}
          onCheckedChange={() => toggle.mutate({ id: todo.id, completed: todo.completed })}
          aria-label={todo.completed ? `Znovu otevřít: ${todo.text}` : `Hotovo: ${todo.text}`}
          className="size-5 rounded-full md:size-4"
        />
      </label>
      <div className="flex min-w-0 flex-1 flex-col gap-1 py-2.5 md:py-2">
        <p className={cn('text-sm leading-snug break-words', todo.completed && 'text-muted-foreground line-through decoration-muted-foreground/60')}>
          {todo.text}
        </p>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
          {todo.automatic && (
            <Badge variant="secondary" className="h-5 gap-1 px-1.5 text-[11px]" title="Vytvořeno automaticky; dokončí se samo po vyřešení">
              <BotIcon className="size-3" aria-hidden="true" />
              auto
            </Badge>
          )}
          {todo.due_on && !todo.completed && (
            <span className={cn('inline-flex items-center gap-1', dueClasses[due])}>
              <CalendarIcon className="size-3" aria-hidden="true" />
              {formatDate(todo.due_on)}
              <span className="text-muted-foreground">({formatDueRelative(todo.due_on)})</span>
            </span>
          )}
          {todo.completed && todo.completed_at && (
            <span className="text-muted-foreground">Hotovo {formatDateTime(todo.completed_at)}</span>
          )}
          {href && (
            <HrefLink href={href} className="inline-flex min-h-6 items-center gap-0.5 font-medium text-primary underline-offset-4 hover:underline">
              {relatedLinkLabels[todo.related_type ?? ''] ?? 'Otevřít'}
              <ChevronRightIcon className="size-3" aria-hidden="true" />
            </HrefLink>
          )}
        </div>
      </div>
      {editable && (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="mt-0.5 shrink-0 text-muted-foreground" aria-label={`Akce úkolu ${todo.text}`} />}
          >
            <EllipsisVerticalIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-40">
            {onEdit && (
              <DropdownMenuItem onClick={() => onEdit(todo)}>
                <PencilIcon />
                Upravit
              </DropdownMenuItem>
            )}
            {onDelete && (
              <DropdownMenuItem variant="destructive" onClick={() => onDelete(todo)}>
                <Trash2Icon />
                Smazat
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </li>
  )
}
