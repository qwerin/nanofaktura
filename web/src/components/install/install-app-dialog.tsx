import { EllipsisVerticalIcon, ShareIcon, SquarePlusIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'

/** Návod na ruční přidání na plochu (iOS a prohlížeče bez systémového dialogu). */
export function InstallAppDialog({
  ios,
  open,
  onOpenChange,
}: {
  ios: boolean
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Nainstalovat aplikaci"
      description="NanoFaktura se pak otevírá z ikony na ploše přes celou obrazovku, bez adresního řádku."
      footer={<Button onClick={() => onOpenChange(false)}>Rozumím</Button>}
    >
      <ol className="flex flex-col gap-4 text-sm">
        {ios ? (
          <>
            <Step n={1} icon={<ShareIcon className="size-5" />}>
              V Safari klepněte dole na <strong>Sdílet</strong>.
            </Step>
            <Step n={2} icon={<SquarePlusIcon className="size-5" />}>
              Sjeďte níž a zvolte <strong>Přidat na plochu</strong>.
            </Step>
            <Step n={3}>
              Potvrďte <strong>Přidat</strong>. Aplikaci pak otevírejte z ikony na ploše.
            </Step>
          </>
        ) : (
          <>
            <Step n={1} icon={<EllipsisVerticalIcon className="size-5" />}>
              Otevřete menu prohlížeče (tři tečky nebo čárky).
            </Step>
            <Step n={2} icon={<SquarePlusIcon className="size-5" />}>
              Zvolte <strong>Instalovat aplikaci</strong> nebo <strong>Přidat na plochu</strong>.
            </Step>
            <Step n={3}>Aplikaci pak otevírejte z ikony na ploše.</Step>
          </>
        )}
      </ol>
    </ResponsiveDialog>
  )
}

function Step({ n, icon, children }: { n: number; icon?: ReactNode; children: ReactNode }) {
  return (
    <li className="flex items-start gap-3">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium">
        {n}
      </span>
      <span className="flex-1 pt-1">{children}</span>
      {icon && <span className="pt-0.5 text-muted-foreground">{icon}</span>}
    </li>
  )
}
