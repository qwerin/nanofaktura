import { useQuery } from '@tanstack/react-query'
import {
  CameraIcon,
  FileCodeIcon,
  FileIcon,
  FileTextIcon,
  PaperclipIcon,
  Trash2Icon,
  UploadIcon,
  XIcon,
} from 'lucide-react'
import { useRef, useState, type DragEvent } from 'react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/errors'
import {
  attachmentQueries,
  attachmentUrl,
  useDeleteAttachment,
  useUploadAttachment,
} from '@/api/queries/attachments'
import type { Attachment, AttachmentOwnerType } from '@/api/types'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { useCurrentAccount } from '@/hooks/use-current-account'
import { formatDateTime } from '@/lib/date'
import { cn } from '@/lib/utils'
import {
  ATTACHMENT_ACCEPT,
  attachmentKind,
  formatFileSize,
  isPreviewableImage,
  RECEIPT_CAPTURE_ACCEPT,
  uploadErrorText,
  validateAttachmentFile,
} from './files'

export interface AttachmentsProps {
  ownerType: AttachmentOwnerType
  ownerId: number
  /** Smí nahrávat a mazat (role owner/admin/member). */
  canEdit: boolean
  title?: string
  /** Text prázdného stavu. */
  emptyText?: string
  className?: string
}

interface UploadState {
  key: string
  name: string
  progress: number
  abort: AbortController
}

/**
 * Přílohy jednoho záznamu (náklad, faktura, kontakt…): náhledy obrázků, ikony PDF/XML,
 * otevření v nové záložce, nahrávání (drag & drop na desktopu, fotoaparát/soubor na mobilu) a mazání.
 */
export function Attachments({
  ownerType,
  ownerId,
  canEdit,
  title = 'Přílohy',
  emptyText = 'Zatím žádné přílohy.',
  className,
}: AttachmentsProps) {
  const { slug } = useCurrentAccount()
  const list = useQuery(attachmentQueries.list(slug, ownerType, ownerId))
  const upload = useUploadAttachment(slug)
  const [uploads, setUploads] = useState<UploadState[]>([])
  const [deleting, setDeleting] = useState<Attachment | null>(null)
  const [dragOver, setDragOver] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)
  const cameraInput = useRef<HTMLInputElement>(null)

  const startUploads = (files: FileList | File[] | null) => {
    if (!files) return
    for (const file of Array.from(files)) {
      const problem = validateAttachmentFile(file)
      if (problem) {
        toast.error(problem)
        continue
      }
      const key = `${file.name}-${file.size}-${Math.random().toString(36).slice(2)}`
      const abort = new AbortController()
      setUploads((u) => [...u, { key, name: file.name, progress: 0, abort }])
      // mutateAsync: callbacky předané do `mutate` by při více souborech najednou doběhly jen u posledního.
      upload
        .mutateAsync({
          ownerType,
          ownerId,
          file,
          signal: abort.signal,
          onProgress: (p) => setUploads((u) => u.map((x) => (x.key === key ? { ...x, progress: p } : x))),
        })
        .then(
          () => toast.success(`Nahráno: ${file.name}`),
          (err: unknown) => {
            if (err instanceof DOMException && err.name === 'AbortError') return
            toast.error(`${file.name}: ${uploadErrorMessage(err)}`)
          },
        )
        .finally(() => setUploads((u) => u.filter((x) => x.key !== key)))
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    if (canEdit) startUploads(e.dataTransfer.files)
  }

  const items = list.data ?? []

  return (
    <section
      className={cn('flex flex-col gap-3', className)}
      aria-label={title}
      onDragOver={
        canEdit
          ? (e) => {
              if (!e.dataTransfer.types.includes('Files')) return
              e.preventDefault()
              setDragOver(true)
            }
          : undefined
      }
      onDragLeave={canEdit ? (e) => !e.currentTarget.contains(e.relatedTarget as Node) && setDragOver(false) : undefined}
      onDrop={canEdit ? onDrop : undefined}
    >
      <div className="flex items-center gap-2">
        <h2 className="flex flex-1 items-center gap-2 text-base font-semibold tracking-tight">
          <PaperclipIcon className="size-4 text-muted-foreground" />
          {title}
          {items.length > 0 && <span className="text-sm font-normal text-muted-foreground">({items.length})</span>}
        </h2>
        {canEdit && (
          <>
            <Button variant="outline" size="sm" className="h-9 md:hidden" onClick={() => cameraInput.current?.click()}>
              <CameraIcon data-icon="inline-start" />
              Vyfotit
            </Button>
            <Button variant="outline" size="sm" className="h-9 md:h-7" onClick={() => fileInput.current?.click()}>
              <UploadIcon data-icon="inline-start" />
              Nahrát
            </Button>
            <input
              ref={fileInput}
              type="file"
              multiple
              accept={ATTACHMENT_ACCEPT}
              className="sr-only"
              tabIndex={-1}
              aria-hidden="true"
              onChange={(e) => {
                startUploads(e.target.files)
                e.target.value = ''
              }}
            />
            <input
              ref={cameraInput}
              type="file"
              accept={RECEIPT_CAPTURE_ACCEPT}
              capture="environment"
              className="sr-only"
              tabIndex={-1}
              aria-hidden="true"
              onChange={(e) => {
                startUploads(e.target.files)
                e.target.value = ''
              }}
            />
          </>
        )}
      </div>

      {uploads.length > 0 && (
        <ul className="flex flex-col gap-2">
          {uploads.map((u) => (
            <li key={u.key} className="flex items-center gap-3 rounded-lg border bg-card px-3 py-2">
              <Spinner className="text-muted-foreground" />
              <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                <span className="truncate text-sm">{u.name}</span>
                <div className="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-valuenow={Math.round(u.progress * 100)} aria-valuemin={0} aria-valuemax={100}>
                  <div className="h-full rounded-full bg-primary transition-[width]" style={{ width: `${Math.round(u.progress * 100)}%` }} />
                </div>
              </div>
              <span className="w-10 text-right text-xs text-muted-foreground tabular-nums">{Math.round(u.progress * 100)} %</span>
              <Button variant="ghost" size="icon-sm" aria-label={`Zrušit nahrávání ${u.name}`} onClick={() => u.abort.abort()}>
                <XIcon />
              </Button>
            </li>
          ))}
        </ul>
      )}

      {list.isPending ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {[0, 1].map((i) => (
            <Skeleton key={i} className="aspect-[4/3] rounded-xl" />
          ))}
        </div>
      ) : list.isError ? (
        <p className="text-sm text-destructive">{errorMessage(list.error)}</p>
      ) : items.length > 0 ? (
        <ul className={cn('grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4', dragOver && 'opacity-60')}>
          {items.map((att) => (
            <li key={att.id}>
              <AttachmentTile
                attachment={att}
                href={attachmentUrl(slug, att.id, { inline: true })}
                onDelete={canEdit ? () => setDeleting(att) : undefined}
              />
            </li>
          ))}
        </ul>
      ) : null}

      {canEdit ? (
        <button
          type="button"
          onClick={() => fileInput.current?.click()}
          className={cn(
            'hidden w-full flex-col items-center justify-center gap-1 rounded-xl border border-dashed px-4 py-6 text-center text-sm text-muted-foreground transition-colors hover:border-ring hover:bg-muted/50 md:flex',
            dragOver && 'border-primary bg-accent text-accent-foreground',
          )}
        >
          <UploadIcon className="mb-1 size-5" />
          <span>
            <span className="font-medium text-foreground">Přetáhněte soubory sem</span> nebo klikněte pro výběr
          </span>
          <span className="text-xs">PDF, obrázky nebo XML, max. 20 MB</span>
        </button>
      ) : null}
      {items.length === 0 && !list.isPending && !list.isError && uploads.length === 0 && (
        <p className={cn('text-sm text-muted-foreground', canEdit && 'md:hidden')}>{emptyText}</p>
      )}

      <DeleteAttachmentDialog attachment={deleting} onClose={() => setDeleting(null)} />
    </section>
  )
}

function uploadErrorMessage(err: unknown): string {
  return (isApiError(err) && uploadErrorText(err.status)) || errorMessage(err)
}

function AttachmentTile({
  attachment,
  href,
  onDelete,
}: {
  attachment: Attachment
  href: string
  onDelete?: () => void
}) {
  const [imgFailed, setImgFailed] = useState(false)
  const kind = attachmentKind(attachment.content_type, attachment.filename)
  const showImage = kind === 'image' && isPreviewableImage(attachment.content_type) && !imgFailed
  const Icon = kind === 'pdf' ? FileTextIcon : kind === 'xml' ? FileCodeIcon : FileIcon

  return (
    <div className="group relative overflow-hidden rounded-xl border bg-card">
      <a
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        className="block focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
        title={`Otevřít ${attachment.filename}`}
      >
        <div className="flex aspect-[4/3] items-center justify-center overflow-hidden bg-muted/60">
          {showImage ? (
            <img
              src={href}
              alt={attachment.filename}
              loading="lazy"
              className="size-full object-cover transition-transform group-hover:scale-[1.02]"
              onError={() => setImgFailed(true)}
            />
          ) : (
            <div className="flex flex-col items-center gap-1.5 text-muted-foreground">
              <Icon className={cn('size-9', kind === 'pdf' && 'text-destructive/80')} strokeWidth={1.5} />
              <span className="text-[11px] font-semibold tracking-wide uppercase">
                {kind === 'other' ? (attachment.filename.split('.').pop() ?? 'soubor') : kind}
              </span>
            </div>
          )}
        </div>
        <div className="flex flex-col gap-0.5 px-3 py-2">
          <span className="truncate text-sm font-medium">{attachment.filename}</span>
          <span className="truncate text-xs text-muted-foreground">
            {formatFileSize(attachment.size)} · {formatDateTime(attachment.created_at)}
          </span>
        </div>
      </a>
      {onDelete && (
        <Button
          variant="secondary"
          size="icon-sm"
          aria-label={`Smazat ${attachment.filename}`}
          onClick={onDelete}
          className="absolute top-2 right-2 size-8 bg-background/85 shadow-sm backdrop-blur hover:text-destructive md:size-7 md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100"
        >
          <Trash2Icon />
        </Button>
      )}
    </div>
  )
}

function DeleteAttachmentDialog({ attachment, onClose }: { attachment: Attachment | null; onClose: () => void }) {
  const { slug } = useCurrentAccount()
  const remove = useDeleteAttachment(slug)
  return (
    <ResponsiveDialog
      open={attachment !== null}
      onOpenChange={(o) => !o && onClose()}
      title="Smazat přílohu?"
      description={
        attachment && (
          <>
            Soubor <strong className="break-all text-foreground">{attachment.filename}</strong> bude trvale odstraněn.
          </>
        )
      }
      footer={
        <>
          <Button
            variant="destructive"
            disabled={remove.isPending}
            onClick={() =>
              attachment &&
              remove.mutate(attachment, {
                onSuccess: () => {
                  toast.success('Příloha smazána')
                  onClose()
                },
              })
            }
          >
            {remove.isPending && <Spinner data-icon="inline-start" />}
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
