// Volby exportu a stahování — sdílené komponentami ExportMenu a hookem useExportMenu.

import { ArchiveIcon, FileCodeIcon, FileSpreadsheetIcon, FileTextIcon, type LucideIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/errors'
import { downloadFile } from './download'
import { exportUrl, pdfZipUrl, type ExportFilters, type ExportKind } from './export-url'

export interface ExportOption {
  id: string
  label: string
  description: string
  icon: LucideIcon
  url: string
  /** Název souboru, když server nepošle Content-Disposition. */
  fallbackName: string
}

export interface ExportGroup {
  label: string
  options: ExportOption[]
}

const kindNames: Record<ExportKind, string> = {
  invoices: 'faktury',
  subjects: 'kontakty',
  expenses: 'naklady',
}

/** Volby exportu pro daný seznam (pro faktury navíc ZIP s PDF, volitelně s ISDOC). */
export function exportGroups(slug: string, kind: ExportKind, filters: ExportFilters): ExportGroup[] {
  const name = kindNames[kind]
  const groups: ExportGroup[] = [
    {
      label: 'Tabulka',
      options: [
        {
          id: 'xlsx',
          label: 'Excel (XLSX)',
          description: 'Pro Excel, Numbers nebo Tabulky Google',
          icon: FileSpreadsheetIcon,
          url: exportUrl(slug, kind, 'xlsx', filters),
          fallbackName: `${name}.xlsx`,
        },
        {
          id: 'csv',
          label: 'CSV',
          description: 'Oddělené středníkem, UTF-8',
          icon: FileTextIcon,
          url: exportUrl(slug, kind, 'csv', filters),
          fallbackName: `${name}.csv`,
        },
      ],
    },
  ]
  if (kind === 'invoices') {
    groups.push({
      label: 'Doklady pro účetní',
      options: [
        {
          id: 'pdf-zip',
          label: 'ZIP s PDF',
          description: 'PDF všech vyfiltrovaných dokladů',
          icon: ArchiveIcon,
          url: pdfZipUrl(slug, filters),
          fallbackName: 'faktury-pdf.zip',
        },
        {
          id: 'pdf-isdoc-zip',
          label: 'ZIP s PDF a ISDOC',
          description: 'Navíc ISDOC pro import do účetnictví',
          icon: FileCodeIcon,
          url: pdfZipUrl(slug, filters, { isdoc: true }),
          fallbackName: 'faktury-pdf-isdoc.zip',
        },
      ],
    })
  }
  return groups
}

export function exportErrorMessage(err: unknown, option: ExportOption): string {
  if (isApiError(err) && err.status === 422 && option.id.includes('zip')) {
    return 'Dokladů je příliš mnoho (nejvýše 5 000) — zužte období ve filtrech.'
  }
  return errorMessage(err)
}

/** Stav stahování: právě stahovaná volba + spuštění s toastem. */
export function useExportDownload() {
  const [busy, setBusy] = useState<string | null>(null)
  const run = async (option: ExportOption) => {
    if (busy) return
    setBusy(option.id)
    const id = toast.loading(`Připravuji ${option.label}…`, {
      description: option.id.includes('zip') ? 'U většího počtu dokladů to může chvíli trvat.' : undefined,
    })
    try {
      const name = await downloadFile(option.url, option.fallbackName)
      toast.success('Export stažen', { id, description: name })
    } catch (err) {
      toast.error('Export se nezdařil', { id, description: exportErrorMessage(err, option) })
    } finally {
      setBusy(null)
    }
  }
  return { busy, run }
}

export type ExportDownload = ReturnType<typeof useExportDownload>

export interface ExportMenuOptions {
  slug: string
  kind: ExportKind
  /** Aktuální filtry seznamu (stejné jako pro API seznamu). */
  filters: ExportFilters
  /** Popisek tlačítka, výchozí „Exportovat“. */
  label?: string
}

/** Jsou nastavené nějaké filtry? (text v menu „podle filtrů“ vs. „celý seznam“) */
export function hasFilters(filters: ExportFilters): boolean {
  return Object.values(filters).some((v) => v !== undefined && v !== '' && v !== null && v !== false)
}
