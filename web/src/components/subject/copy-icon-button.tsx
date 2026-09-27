import { CheckIcon, CopyIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/** Ikonové tlačítko „zkopírovat“ (44 px na mobilu). */
export function CopyIconButton({ value, label, className }: { value: string; label: string; className?: string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => setCopied(false), 1500)
    return () => clearTimeout(t)
  }, [copied])

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={copied ? 'Zkopírováno' : label}
      title={label}
      className={cn('shrink-0 text-muted-foreground', className)}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value)
          setCopied(true)
          toast.success('Zkopírováno do schránky', { duration: 1500 })
        } catch {
          toast.error('Kopírování se nezdařilo — označte text a zkopírujte ho ručně.')
        }
      }}
    >
      {copied ? <CheckIcon className="text-success" /> : <CopyIcon />}
    </Button>
  )
}
