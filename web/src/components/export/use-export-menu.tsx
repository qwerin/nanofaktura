import { DownloadIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import type { PageAction } from '@/components/page-header'
import { useIsMobile } from '@/hooks/use-mobile'
import { ExportDesktopTrigger, ExportDrawer } from './export-menu'
import { exportGroups, hasFilters, useExportDownload, type ExportMenuOptions } from './export-options'

/**
 * Akce „Exportovat“ pro `PageHeader.actions`. Na desktopu tlačítko s rozbalovacím menu,
 * na mobilu položka v menu „…“ otevírající Drawer. `drawer` vykreslete kdekoli na stránce.
 *
 *   const exportMenu = useExportMenu({ slug, kind: 'invoices', filters })
 *   <PageHeader actions={[newAction, exportMenu.action]} />
 *   {exportMenu.drawer}
 */
export function useExportMenu({ slug, kind, filters, label = 'Exportovat' }: ExportMenuOptions): {
  action: PageAction
  drawer: ReactNode
} {
  const isMobile = useIsMobile()
  const [open, setOpen] = useState(false)
  const download = useExportDownload()
  const groups = exportGroups(slug, kind, filters)
  const filtered = hasFilters(filters)

  const action: PageAction = isMobile
    ? { label, icon: DownloadIcon, onClick: () => setOpen(true) }
    : {
        label,
        icon: DownloadIcon,
        variant: 'outline',
        render: <ExportDesktopTrigger groups={groups} download={download} filtered={filtered} />,
      }

  const drawer = isMobile ? (
    <ExportDrawer open={open} onOpenChange={setOpen} groups={groups} download={download} filtered={filtered} />
  ) : null

  return { action, drawer }
}
