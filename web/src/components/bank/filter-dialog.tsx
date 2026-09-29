import { useId, useState } from 'react'
import type { BankAccount } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { BankSearch } from './format'

const ALL = '__all'

interface BankFilterDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  value: BankSearch
  accounts: BankAccount[]
  onApply: (next: BankSearch) => void
}

/** Filtry pohybů (účet, směr, období). Drawer na mobilu, hodnoty jdou do URL. */
export function BankFilterDialog(props: BankFilterDialogProps) {
  return (
    <ResponsiveDialog open={props.open} onOpenChange={props.onOpenChange} title="Filtry" description="Zúžte seznam pohybů na účtu.">
      {props.open && <FilterBody key={JSON.stringify(props.value)} {...props} />}
    </ResponsiveDialog>
  )
}

function FilterBody({ value, accounts, onApply, onOpenChange }: BankFilterDialogProps) {
  const id = useId()
  const [draft, setDraft] = useState<BankSearch>(value)
  const accountItems = [{ value: ALL, label: 'Všechny účty' }, ...accounts.map((a) => ({ value: String(a.id), label: a.name }))]

  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault()
        onApply(draft)
        onOpenChange(false)
      }}
    >
      {accounts.length > 1 && (
        <Field>
          <FieldLabel htmlFor={`${id}-account`}>Účet</FieldLabel>
          <Select
            items={accountItems}
            value={draft.account ? String(draft.account) : ALL}
            onValueChange={(v) => setDraft((d) => ({ ...d, account: !v || v === ALL ? undefined : Number(v) }))}
          >
            <SelectTrigger id={`${id}-account`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {accountItems.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      )}

      <Field>
        <FieldLabel>Směr</FieldLabel>
        <Tabs
          value={draft.direction ?? ALL}
          onValueChange={(v) => setDraft((d) => ({ ...d, direction: v === ALL ? undefined : (v as BankSearch['direction']) }))}
        >
          <TabsList className="h-11! w-full md:h-8!" aria-label="Směr platby">
            <TabsTrigger value={ALL}>Vše</TabsTrigger>
            <TabsTrigger value="in">Příchozí</TabsTrigger>
            <TabsTrigger value="out">Odchozí</TabsTrigger>
          </TabsList>
        </Tabs>
      </Field>

      <div className="grid grid-cols-2 gap-3">
        <Field>
          <FieldLabel htmlFor={`${id}-since`}>Zaúčtováno od</FieldLabel>
          <Input
            id={`${id}-since`}
            type="date"
            value={draft.since ?? ''}
            max={draft.until}
            onChange={(e) => setDraft((d) => ({ ...d, since: e.target.value || undefined }))}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-until`}>do</FieldLabel>
          <Input
            id={`${id}-until`}
            type="date"
            value={draft.until ?? ''}
            min={draft.since}
            onChange={(e) => setDraft((d) => ({ ...d, until: e.target.value || undefined }))}
          />
        </Field>
      </div>

      <div className="flex flex-col-reverse gap-2 md:flex-row md:justify-end">
        <Button
          type="button"
          variant="outline"
          onClick={() => setDraft((d) => ({ tab: d.tab, query: d.query }))}
        >
          Vymazat
        </Button>
        <Button type="submit">Použít filtry</Button>
      </div>
    </form>
  )
}
