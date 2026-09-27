import { zodResolver } from '@hookform/resolvers/zod'
import { CircleAlertIcon, CircleCheckIcon } from 'lucide-react'
import { useForm, useWatch } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'
import { applyProblemToForm, errorMessage } from '@/api/errors'
import { useCreateBankAccount, useUpdateBankAccount } from '@/api/queries/bank-accounts'
import type { BankAccount, CreateBankAccountInput, UpdateBankAccountInput } from '@/api/types'
import { SelectField, SwitchField, TextField, type SelectOption } from '@/components/form/fields'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  bankName,
  bankSwift,
  checkCzechAccount,
  czechAccountErrorMessage,
  czechAccountToIban,
  formatIban,
  isValidIban,
  isValidSwift,
} from '@/lib/bank'

const currencyOptions: SelectOption[] = ['CZK', 'EUR', 'USD', 'GBP', 'PLN'].map((c) => ({ value: c, label: c }))

const schema = z
  .object({
    kind: z.enum(['czech', 'foreign']),
    name: z.string().trim().min(1, 'Pojmenujte účet').max(100, 'Nejvýše 100 znaků'),
    number: z.string(),
    iban: z.string(),
    swift_bic: z.string(),
    currency: z.string().regex(/^[A-Z]{3}$/, 'Vyberte měnu'),
    is_default: z.boolean(),
  })
  .superRefine((v, ctx) => {
    if (v.kind === 'czech') {
      const r = checkCzechAccount(v.number)
      if ('error' in r) {
        ctx.addIssue({
          code: 'custom',
          path: ['number'],
          message: v.number.trim() === '' ? 'Zadejte číslo účtu' : czechAccountErrorMessage(r.error),
        })
      }
    } else {
      if (!isValidIban(v.iban)) {
        ctx.addIssue({ code: 'custom', path: ['iban'], message: v.iban.trim() === '' ? 'Zadejte IBAN' : 'Neplatný IBAN — zkontrolujte překlep' })
      }
      if (v.swift_bic.trim() !== '' && !isValidSwift(v.swift_bic)) {
        ctx.addIssue({ code: 'custom', path: ['swift_bic'], message: 'SWIFT/BIC má 8 nebo 11 znaků, např. GIBACZPX' })
      }
    }
  })
type Values = z.infer<typeof schema>

const SERVER_FIELDS = ['name', 'number', 'iban', 'swift_bic', 'currency', 'is_default']

function toFormValues(a: BankAccount | undefined, defaultCurrency: string, isFirst: boolean): Values {
  return {
    kind: a && !a.number && a.iban ? 'foreign' : 'czech',
    name: a?.name ?? (isFirst ? 'Hlavní účet' : ''),
    number: a?.number ?? '',
    iban: a?.number ? '' : (a?.iban ? formatIban(a.iban) : ''),
    swift_bic: a?.number ? '' : (a?.swift_bic ?? ''),
    currency: a?.currency ?? defaultCurrency,
    is_default: a?.is_default ?? isFirst,
  }
}

function toBody(v: Values): CreateBankAccountInput & UpdateBankAccountInput {
  const base = { name: v.name.trim(), currency: v.currency, is_default: v.is_default }
  if (v.kind === 'czech') return { ...base, number: v.number.replace(/\s+/g, '') }
  return {
    ...base,
    number: '',
    iban: v.iban.replace(/\s+/g, '').toUpperCase(),
    swift_bic: v.swift_bic.trim().toUpperCase(),
  }
}

interface BankAccountFormProps {
  slug: string
  /** Úprava existujícího účtu; bez něj se zakládá nový. */
  account?: BankAccount
  defaultCurrency: string
  /** První účet firmy — předvyplní název a je vždy výchozí. */
  isFirst?: boolean
  formId: string
  onSaved: (account: BankAccount) => void
  /** Skrýt měnu a „výchozí“ (onboarding). */
  simple?: boolean
}

/**
 * Formulář bankovního účtu. Český účet „předčíslí-číslo/kód banky“ se živě ověřuje (mod 11)
 * a ukazuje dopočítaný IBAN; zahraniční účet se zadává jako IBAN + SWIFT.
 * Tlačítko pro odeslání je mimo formulář (`form={formId}`).
 */
export function BankAccountForm({
  slug,
  account,
  defaultCurrency,
  isFirst = false,
  formId,
  onSaved,
  simple,
}: BankAccountFormProps) {
  const create = useCreateBankAccount(slug)
  const update = useUpdateBankAccount(slug)
  const pending = create.isPending || update.isPending

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: toFormValues(account, defaultCurrency, isFirst),
    mode: 'onTouched',
  })
  const { control } = form
  const [kind, number, iban, currency] = useWatch({ control, name: ['kind', 'number', 'iban', 'currency'] })

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      const body = toBody(values)
      const saved = account ? await update.mutateAsync({ id: account.id, body }) : await create.mutateAsync(body)
      toast.success(account ? 'Bankovní účet uložen' : 'Bankovní účet přidán')
      onSaved(saved)
    } catch (err) {
      if (applyProblemToForm(err, form.setError, SERVER_FIELDS)) return
      form.setError('root', { message: errorMessage(err) })
    }
  })

  return (
    <form id={formId} onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      <fieldset disabled={pending} className="contents">
        <Tabs
          value={kind}
          onValueChange={(v) => {
            form.setValue('kind', v as Values['kind'], { shouldDirty: true })
            form.clearErrors()
          }}
        >
          <TabsList className="h-11! w-full md:h-8!" aria-label="Typ účtu">
            <TabsTrigger value="czech">Český účet</TabsTrigger>
            <TabsTrigger value="foreign">Zahraniční (IBAN)</TabsTrigger>
          </TabsList>
        </Tabs>

        {kind === 'czech' ? (
          <div className="flex flex-col gap-2">
            <TextField
              control={control}
              name="number"
              label="Číslo účtu"
              placeholder="19-2000145399/0800"
              inputMode="text"
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              enterKeyHint="next"
              autoFocus={!account}
            />
            <CzechAccountPreview value={number} hideError={Boolean(form.formState.errors.number)} />
          </div>
        ) : (
          <>
            <div className="flex flex-col gap-2">
              <TextField
                control={control}
                name="iban"
                label="IBAN"
                placeholder="SK31 1200 0000 1987 4263 7541"
                autoCapitalize="characters"
                autoComplete="off"
                spellCheck={false}
                autoFocus={!account}
              />
              {iban.replace(/\s+/g, '').length >= 15 && isValidIban(iban) && (
                <p className="flex items-center gap-1.5 text-sm text-success">
                  <CircleCheckIcon className="size-4 shrink-0" />
                  <span className="font-mono">{formatIban(iban)}</span>
                </p>
              )}
            </div>
            <TextField
              control={control}
              name="swift_bic"
              label="SWIFT/BIC"
              description="Pro platby ze zahraničí. U účtů v SEPA stačí IBAN."
              autoCapitalize="characters"
              autoComplete="off"
              spellCheck={false}
              maxLength={11}
            />
          </>
        )}

        <TextField
          control={control}
          name="name"
          label="Název účtu"
          description="Jen pro vás, např. „Hlavní účet“ nebo „Eurový účet“."
          autoComplete="off"
          enterKeyHint="done"
        />

        {!simple && (
          <>
            <SelectField control={control} name="currency" label="Měna účtu" options={currencyOptions} />
            <SwitchField
              control={control}
              name="is_default"
              label={`Výchozí účet pro ${currency}`}
              description="Předvyplní se na nové faktury v této měně."
              disabled={isFirst || account?.is_default}
            />
          </>
        )}

        {form.formState.errors.root && (
          <p className="flex items-center gap-1.5 text-sm text-destructive" role="alert">
            <CircleAlertIcon className="size-4 shrink-0" />
            {form.formState.errors.root.message}
          </p>
        )}
      </fieldset>
    </form>
  )
}

/** Živý náhled: banka + IBAN z českého čísla, nebo nápověda / chyba. */
function CzechAccountPreview({ value, hideError }: { value: string; hideError: boolean }) {
  const trimmed = value.replace(/\s+/g, '')
  if (trimmed === '') {
    return <p className="text-sm text-muted-foreground">Ve tvaru předčíslí-číslo/kód banky. IBAN dopočítáme.</p>
  }
  const r = checkCzechAccount(trimmed)
  if ('account' in r) {
    const iban = czechAccountToIban(r.account)
    const bank = bankName(r.account.bank)
    const swift = bankSwift(r.account.bank)
    return (
      <div className="flex items-start gap-2 rounded-lg bg-success/10 px-3 py-2 text-sm" aria-live="polite">
        <CircleCheckIcon className="mt-0.5 size-4 shrink-0 text-success" />
        <div className="min-w-0">
          <p className="font-medium">{bank ?? `Banka ${r.account.bank}`}</p>
          <p className="font-mono text-xs break-all text-muted-foreground">
            {formatIban(iban)}
            {swift && ` · ${swift}`}
          </p>
        </div>
      </div>
    )
  }
  // Rozepsané číslo (ještě bez kódu banky) nehlásíme jako chybu.
  if (hideError) return null
  if (r.error === 'format' && !/\/\d{4}$/.test(trimmed)) {
    return <p className="text-sm text-muted-foreground">Doplňte lomítko a čtyřmístný kód banky.</p>
  }
  return (
    <p className="flex items-start gap-1.5 text-sm text-destructive" aria-live="polite">
      <CircleAlertIcon className="mt-0.5 size-4 shrink-0" />
      {czechAccountErrorMessage(r.error)}
    </p>
  )
}
