import { useQuery } from '@tanstack/react-query'
import { XIcon } from 'lucide-react'
import { useState } from 'react'
import { expenseQueries, type SupplierOption } from '@/api/queries/expenses'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { AutocompleteInput } from './autocomplete-input'
import type { ExpenseSearch } from './expense-search'
import { expenseStatusOptions } from './format'
import { useDebouncedValue } from './use-debounced-value'

interface ExpenseFilterDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  value: ExpenseSearch
  /** Název dodavatele pro `value.subject_id` (už načtený stránkou). */
  supplierName?: string
  onApply: (next: ExpenseSearch) => void
}

const ALL = '__all'
const statusItems = [{ value: ALL, label: 'Všechny stavy' }, ...expenseStatusOptions]

/** Filtry seznamu nákladů (stav, kategorie, dodavatel, období). Drawer na mobilu. */
export function ExpenseFilterDialog(props: ExpenseFilterDialogProps) {
  return (
    <ResponsiveDialog open={props.open} onOpenChange={props.onOpenChange} title="Filtry" description="Zúžte seznam nákladů.">
      {/* Klíč: při každém otevření začít z aktuálních filtrů v URL. */}
      {props.open && <FilterBody key={JSON.stringify(props.value)} {...props} />}
    </ResponsiveDialog>
  )
}

function FilterBody({ value, supplierName, onApply, onOpenChange }: ExpenseFilterDialogProps) {
  const { slug } = useCurrentAccount()
  const [draft, setDraft] = useState<ExpenseSearch>(value)
  const [supplierText, setSupplierText] = useState(value.subject_id ? (supplierName ?? '') : '')
  const [categoryText, setCategoryText] = useState(value.category ?? '')
  const supplierQuery = useDebouncedValue(supplierText.trim())
  const categoryQuery = useDebouncedValue(categoryText.trim(), 200)
  const suppliers = useQuery({ ...expenseQueries.supplierSearch(slug, supplierQuery), enabled: !draft.subject_id && supplierQuery !== '' })
  const categories = useQuery(expenseQueries.categories(slug, categoryQuery))

  const apply = () => {
    onApply({ ...draft, category: categoryText.trim() || undefined })
    onOpenChange(false)
  }

  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        apply()
      }}
    >
      <Field>
        <FieldLabel htmlFor="filter-status">Stav</FieldLabel>
        <Select
          items={statusItems}
          value={draft.status ?? ALL}
          onValueChange={(v) => setDraft((d) => ({ ...d, status: v === ALL || !v ? undefined : (v as ExpenseSearch['status']) }))}
        >
          <SelectTrigger id="filter-status" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {statusItems.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      <Field>
        <FieldLabel htmlFor="filter-supplier">Dodavatel</FieldLabel>
        {draft.subject_id ? (
          <div className="flex h-11 items-center gap-2 rounded-lg border bg-muted/40 pr-1 pl-3 md:h-8">
            <span className="min-w-0 flex-1 truncate text-sm">{supplierText || 'Vybraný kontakt'}</span>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Zrušit výběr dodavatele"
              onClick={() => {
                setDraft((d) => ({ ...d, subject_id: undefined }))
                setSupplierText('')
              }}
            >
              <XIcon />
            </Button>
          </div>
        ) : (
          <AutocompleteInput<SupplierOption>
            id="filter-supplier"
            value={supplierText}
            onValueChange={setSupplierText}
            onSelect={(s) => {
              setSupplierText(s.name)
              setDraft((d) => ({ ...d, subject_id: s.id }))
            }}
            items={supplierQuery ? (suppliers.data ?? []) : []}
            loading={suppliers.isFetching}
            itemToString={(s) => s.name}
            itemKey={(s) => s.id}
            placeholder="Hledat v kontaktech"
            inputProps={{ autoComplete: 'off' }}
          />
        )}
      </Field>

      <Field>
        <FieldLabel htmlFor="filter-category">Kategorie</FieldLabel>
        <AutocompleteInput<string>
          id="filter-category"
          value={categoryText}
          onValueChange={setCategoryText}
          onSelect={setCategoryText}
          items={(categories.data ?? []).filter((c) => c !== categoryText)}
          itemToString={(c) => c}
          itemKey={(c) => c}
          placeholder="Všechny kategorie"
          inputProps={{ autoComplete: 'off' }}
        />
      </Field>

      <div className="grid grid-cols-2 gap-3">
        <Field>
          <FieldLabel htmlFor="filter-since">Vystaveno od</FieldLabel>
          <Input id="filter-since" type="date" value={draft.since ?? ''} onChange={(e) => setDraft((d) => ({ ...d, since: e.target.value || undefined }))} />
        </Field>
        <Field>
          <FieldLabel htmlFor="filter-until">do</FieldLabel>
          <Input id="filter-until" type="date" value={draft.until ?? ''} onChange={(e) => setDraft((d) => ({ ...d, until: e.target.value || undefined }))} />
        </Field>
      </div>

      <div className="flex flex-col-reverse gap-2 pt-1 sm:flex-row sm:justify-end">
        <Button
          type="button"
          variant="ghost"
          onClick={() => {
            onApply({ query: value.query })
            onOpenChange(false)
          }}
        >
          Zrušit filtry
        </Button>
        <Button type="submit">Použít</Button>
      </div>
    </form>
  )
}
