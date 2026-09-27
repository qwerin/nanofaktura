import { useBlocker } from '@tanstack/react-router'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'

/**
 * Upozornění na neuložené změny při odchodu ze stránky (navigace v aplikaci i zavření záložky).
 * `when` = formulář má neuložené změny a právě se neukládá.
 */
export function UnsavedChangesGuard({ when }: { when: boolean }) {
  const blocker = useBlocker({
    shouldBlockFn: () => when,
    enableBeforeUnload: () => when,
    withResolver: true,
  })

  return (
    <ResponsiveDialog
      open={blocker.status === 'blocked'}
      onOpenChange={(open) => !open && blocker.reset?.()}
      title="Zahodit neuložené změny?"
      description="Formulář obsahuje změny, které ještě nejsou uložené."
      footer={
        <>
          <Button variant="destructive" onClick={() => blocker.proceed?.()}>
            Zahodit změny
          </Button>
          <Button variant="outline" onClick={() => blocker.reset?.()}>
            Pokračovat v úpravách
          </Button>
        </>
      }
    />
  )
}
