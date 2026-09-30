import { zodResolver } from '@hookform/resolvers/zod'
import { PlusIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateTodo, useDeleteTodo, useUpdateTodo } from '@/api/queries/todos'
import type { Todo } from '@/api/types'
import { TextareaField, TextField } from '@/components/form/fields'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

const TEXT_MAX = 500

/** Rychlé přidání ručního úkolu: text (+ volitelně termín), Enter uloží. */
export function TodoQuickAdd({ slug, className }: { slug: string; className?: string }) {
  const create = useCreateTodo(slug)
  const [text, setText] = useState('')
  const [dueOn, setDueOn] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const submit = () => {
    const t = text.trim()
    if (!t || create.isPending) return
    create.mutate(
      { text: t.slice(0, TEXT_MAX), ...(dueOn ? { due_on: dueOn } : {}) },
      {
        onSuccess: () => {
          setText('')
          setDueOn('')
          inputRef.current?.focus()
        },
        onError: (err) => toast.error(errorMessage(err)),
      },
    )
  }

  return (
    <form
      className={cn('flex flex-col gap-2 sm:flex-row', className)}
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <Input
        ref={inputRef}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Přidat úkol… (Enter uloží)"
        aria-label="Nový úkol"
        maxLength={TEXT_MAX}
        enterKeyHint="done"
        autoComplete="off"
        className="flex-1"
      />
      <div className="flex gap-2">
        <Input
          type="date"
          value={dueOn}
          onChange={(e) => setDueOn(e.target.value)}
          aria-label="Termín (nepovinné)"
          title="Termín (nepovinné)"
          className="flex-1 sm:w-40 sm:flex-none"
        />
        <Button type="submit" disabled={!text.trim() || create.isPending} className="shrink-0">
          {create.isPending ? <Spinner data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
          Přidat
        </Button>
      </div>
    </form>
  )
}

const editSchema = z.object({
  text: z.string().trim().min(1, 'Napište, co je potřeba udělat').max(TEXT_MAX, `Nejvýše ${TEXT_MAX} znaků`),
  due_on: z.string(),
})
type EditValues = z.infer<typeof editSchema>

export function EditTodoDialog({ slug, todo, onClose }: { slug: string; todo: Todo | null; onClose: () => void }) {
  const update = useUpdateTodo(slug)
  const form = useForm<EditValues>({ resolver: zodResolver(editSchema), defaultValues: { text: '', due_on: '' } })

  useEffect(() => {
    if (todo) form.reset({ text: todo.text, due_on: todo.due_on ?? '' })
  }, [todo, form])

  const onSubmit = form.handleSubmit(async (values) => {
    if (!todo) return
    try {
      // "" termín smaže
      await update.mutateAsync({ id: todo.id, body: { text: values.text, due_on: values.due_on } })
      toast.success('Úkol uložen')
      onClose()
    } catch (err) {
      if (!applyProblemToForm(err, form.setError)) toast.error(errorMessage(err))
    }
  })

  return (
    <ResponsiveDialog
      open={todo !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Upravit úkol"
      footer={
        <>
          <Button type="submit" form="edit-todo-form" disabled={update.isPending}>
            {update.isPending && <Spinner data-icon="inline-start" />}
            Uložit
          </Button>
          <Button variant="outline" onClick={onClose}>
            Zrušit
          </Button>
        </>
      }
    >
      <form id="edit-todo-form" onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
        <TextareaField control={form.control} name="text" label="Úkol" rows={3} />
        <TextField control={form.control} name="due_on" label="Termín" type="date" description="Nepovinné — prázdné pole termín odebere." />
      </form>
    </ResponsiveDialog>
  )
}

export function DeleteTodoDialog({ slug, todo, onClose }: { slug: string; todo: Todo | null; onClose: () => void }) {
  const del = useDeleteTodo(slug)
  return (
    <ResponsiveDialog
      open={todo !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Smazat úkol?"
      description={todo && <>„{todo.text}“ bude trvale odstraněn.</>}
      footer={
        <>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={() =>
              todo &&
              del.mutate(todo, {
                onSuccess: () => {
                  toast.success('Úkol smazán')
                  onClose()
                },
              })
            }
          >
            {del.isPending && <Spinner data-icon="inline-start" />}
            Smazat
          </Button>
          <Button variant="outline" onClick={onClose}>
            Ponechat
          </Button>
        </>
      }
    />
  )
}
