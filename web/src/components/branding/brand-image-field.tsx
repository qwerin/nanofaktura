import { ImageIcon, ImageOffIcon, RefreshCwIcon, Trash2Icon, UploadIcon } from 'lucide-react'
import { useRef, useState, type DragEvent } from 'react'
import { toast } from 'sonner'
import { errorMessage, isApiError } from '@/api/errors'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import {
  BRANDING_ACCEPT,
  brandingImageUrl,
  useSetBrandingImage,
  validateBrandingFile,
  type BrandingKind,
} from './branding-api'

const texts: Record<BrandingKind, { noun: string; upload: string; done: string; removed: string; hint: string }> = {
  logo: {
    noun: 'logo',
    upload: 'Nahrát logo',
    done: 'Logo uloženo',
    removed: 'Logo odebráno',
    hint: 'Nejlépe PNG s průhledným pozadím, na šířku, aspoň 600 px.',
  },
  stamp: {
    noun: 'podpis',
    upload: 'Nahrát podpis',
    done: 'Podpis uložen',
    removed: 'Podpis odebrán',
    hint: 'Sken podpisu nebo razítka, ideálně PNG s průhledným pozadím.',
  },
}

function uploadError(err: unknown): string {
  if (isApiError(err)) {
    if (err.status === 413) return 'Obrázek je příliš velký.'
    if (err.status === 415 || err.status === 422) return 'Tento obrázek nejde použít. Nahrajte PNG nebo JPEG.'
    if (err.status === 403) return 'Vzhled dokladů může měnit jen vlastník nebo administrátor.'
  }
  return errorMessage(err)
}

/** Nahrání / náhled / výměna / odebrání loga nebo podpisu. */
export function BrandImageField({
  slug,
  kind,
  attachmentId,
  canEdit,
}: {
  slug: string
  kind: BrandingKind
  attachmentId: number | undefined
  canEdit: boolean
}) {
  const t = texts[kind]
  const inputRef = useRef<HTMLInputElement>(null)
  const set = useSetBrandingImage(slug)
  const [dragOver, setDragOver] = useState(false)
  const [broken, setBroken] = useState<number | null>(null)
  const busy = set.isPending && set.variables?.kind === kind
  const removing = busy && set.variables?.file === null
  const hasImage = Boolean(attachmentId)

  const pick = () => inputRef.current?.click()

  const upload = (file: File | undefined) => {
    if (!file) return
    const problem = validateBrandingFile(file)
    if (problem) {
      toast.error(problem)
      return
    }
    set.mutate(
      { kind, file, previousId: attachmentId },
      { onSuccess: () => toast.success(t.done), onError: (err) => toast.error(uploadError(err)) },
    )
  }

  const remove = () =>
    set.mutate(
      { kind, file: null, previousId: attachmentId },
      { onSuccess: () => toast.success(t.removed), onError: (err) => toast.error(uploadError(err)) },
    )

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragOver(false)
    if (canEdit && !busy) upload(e.dataTransfer.files[0])
  }

  return (
    <div className="flex flex-col gap-3">
      <div
        onDragOver={(e) => {
          if (!canEdit) return
          e.preventDefault()
          setDragOver(true)
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
        className={cn(
          'relative flex h-40 items-center justify-center overflow-hidden rounded-xl border bg-[repeating-conic-gradient(var(--muted)_0_25%,transparent_0_50%)] bg-[length:16px_16px]',
          !hasImage && 'border-dashed bg-none',
          dragOver && 'border-primary bg-primary/5',
        )}
      >
        {hasImage && attachmentId && broken !== attachmentId ? (
          <img
            key={attachmentId}
            src={brandingImageUrl(slug, attachmentId)}
            alt={kind === 'logo' ? 'Logo firmy' : 'Podpis / razítko'}
            className="max-h-full max-w-full object-contain p-4"
            onError={() => setBroken(attachmentId)}
          />
        ) : hasImage ? (
          <div className="flex flex-col items-center gap-2 text-sm text-muted-foreground">
            <ImageOffIcon className="size-6" />
            Obrázek se nepodařilo načíst
          </div>
        ) : canEdit ? (
          <button
            type="button"
            onClick={pick}
            disabled={busy}
            className="flex size-full flex-col items-center justify-center gap-2 px-4 text-center text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
          >
            {busy ? <Spinner className="size-6" /> : <UploadIcon className="size-6" />}
            <span className="font-medium text-foreground">{t.upload}</span>
            <span className="max-md:hidden">nebo sem přetáhněte obrázek</span>
            <span>PNG nebo JPEG</span>
          </button>
        ) : (
          <div className="flex flex-col items-center gap-2 text-sm text-muted-foreground">
            <ImageIcon className="size-6" />
            Zatím nenahráno
          </div>
        )}
        {busy && hasImage && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/70">
            <Spinner className="size-6" />
          </div>
        )}
      </div>

      {canEdit && (
        <>
          <input
            ref={inputRef}
            type="file"
            accept={BRANDING_ACCEPT}
            className="sr-only"
            tabIndex={-1}
            aria-hidden="true"
            onChange={(e) => {
              upload(e.target.files?.[0])
              e.target.value = ''
            }}
          />
          {hasImage ? (
            <div className="flex gap-2">
              <Button variant="outline" className="max-sm:flex-1" onClick={pick} disabled={busy}>
                <RefreshCwIcon data-icon="inline-start" />
                Nahradit
              </Button>
              <Button
                variant="ghost"
                className="text-destructive hover:text-destructive max-sm:flex-1"
                onClick={remove}
                disabled={busy}
              >
                {removing ? <Spinner data-icon="inline-start" /> : <Trash2Icon data-icon="inline-start" />}
                Odebrat
              </Button>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t.hint}</p>
          )}
        </>
      )}
    </div>
  )
}
