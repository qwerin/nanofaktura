import { CameraIcon, FileCodeIcon, FileIcon, FileTextIcon, PaperclipIcon, XIcon } from 'lucide-react'
import { useEffect, useMemo, useRef } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  ATTACHMENT_ACCEPT,
  attachmentKind,
  formatFileSize,
  isPreviewableImage,
  RECEIPT_CAPTURE_ACCEPT,
  validateAttachmentFile,
} from './files'

interface PendingFilesProps {
  files: File[]
  onChange: (files: File[]) => void
  /** Popisek hlavního tlačítka (focení na mobilu). */
  captureLabel?: string
  hint?: string
  className?: string
  disabled?: boolean
}

/**
 * Výběr souborů před uložením záznamu (např. účtenka k novému nákladu). Soubory se nahrají
 * až po vytvoření vlastníka (`uploadAttachment`). Na mobilu nabízí přímo fotoaparát.
 */
export function PendingFiles({
  files,
  onChange,
  captureLabel = 'Vyfotit doklad',
  hint = 'Fotka nebo PDF účtenky se uloží jako příloha nákladu.',
  className,
  disabled,
}: PendingFilesProps) {
  const cameraInput = useRef<HTMLInputElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const add = (list: FileList | null) => {
    if (!list) return
    const accepted: File[] = []
    for (const f of Array.from(list)) {
      const problem = validateAttachmentFile(f)
      if (problem) toast.error(problem)
      else accepted.push(f)
    }
    if (accepted.length) onChange([...files, ...accepted])
  }

  return (
    <div className={cn('flex flex-col gap-3', className)}>
      <div className="grid grid-cols-2 gap-2 md:flex">
        <Button
          type="button"
          variant="outline"
          className="h-14 flex-col gap-0.5 md:hidden"
          disabled={disabled}
          onClick={() => cameraInput.current?.click()}
        >
          <CameraIcon className="size-5" />
          <span className="text-xs">{captureLabel}</span>
        </Button>
        <Button
          type="button"
          variant="outline"
          className="h-14 flex-col gap-0.5 md:h-8 md:flex-row md:gap-1.5"
          disabled={disabled}
          onClick={() => fileInput.current?.click()}
        >
          <PaperclipIcon className="size-5 md:size-4" />
          <span className="text-xs md:text-sm">Přiložit soubor</span>
        </Button>
      </div>
      <input
        ref={cameraInput}
        type="file"
        accept={RECEIPT_CAPTURE_ACCEPT}
        capture="environment"
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => {
          add(e.target.files)
          e.target.value = ''
        }}
      />
      <input
        ref={fileInput}
        type="file"
        multiple
        accept={ATTACHMENT_ACCEPT}
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
        onChange={(e) => {
          add(e.target.files)
          e.target.value = ''
        }}
      />
      {files.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {files.map((f, i) => (
            <PendingFileRow key={`${f.name}-${f.size}-${f.lastModified}-${i}`} file={f} onRemove={() => onChange(files.filter((_, j) => j !== i))} disabled={disabled} />
          ))}
        </ul>
      ) : (
        hint && <p className="text-sm text-muted-foreground">{hint}</p>
      )}
    </div>
  )
}

function PendingFileRow({ file, onRemove, disabled }: { file: File; onRemove: () => void; disabled?: boolean }) {
  const kind = attachmentKind(file.type, file.name)
  const previewable = kind === 'image' && isPreviewableImage(file.type)
  const url = useMemo(() => (previewable ? URL.createObjectURL(file) : null), [file, previewable])
  useEffect(() => () => {
    if (url) URL.revokeObjectURL(url)
  }, [url])
  const Icon = kind === 'pdf' ? FileTextIcon : kind === 'xml' ? FileCodeIcon : FileIcon

  return (
    <li className="flex items-center gap-3 rounded-lg border bg-card p-2">
      <div className="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-muted">
        {url ? <img src={url} alt="" className="size-full object-cover" /> : <Icon className="size-5 text-muted-foreground" />}
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-sm font-medium">{file.name}</span>
        <span className="text-xs text-muted-foreground">{formatFileSize(file.size)}</span>
      </div>
      <Button type="button" variant="ghost" size="icon" aria-label={`Odebrat ${file.name}`} onClick={onRemove} disabled={disabled}>
        <XIcon />
      </Button>
    </li>
  )
}
