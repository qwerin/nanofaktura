import { useState } from 'react'
import { toast } from 'sonner'
import { useInstallApp } from '@/hooks/use-install-app'
import { InstallAppDialog } from './install-app-dialog'

/**
 * „Nainstalovat aplikaci“ pro menu: `available` = má smysl nabízet, `run` spustí
 * systémový dialog nebo otevře návod; `dialog` vykreslete vedle menu.
 */
export function useInstallAction() {
  const { mode, install } = useInstallApp()
  const [helpOpen, setHelpOpen] = useState(false)
  const run = () => {
    if (mode === 'prompt') {
      void install().then((ok) => ok && toast.success('Aplikace se instaluje na plochu'))
    } else {
      setHelpOpen(true)
    }
  }
  return {
    available: mode === 'prompt' || mode === 'ios' || mode === 'manual',
    run,
    dialog: <InstallAppDialog ios={mode === 'ios'} open={helpOpen} onOpenChange={setHelpOpen} />,
  }
}
