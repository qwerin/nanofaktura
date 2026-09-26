import { CheckIcon, CopyIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'

/** Zkopíruje text do schránky, krátce ukáže „Zkopírováno“. */
export function CopyButton({ value, label = 'Kopírovat', className }: { value: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const t = setTimeout(() => setCopied(false), 2000)
    return () => clearTimeout(t)
  }, [copied])

  return (
    <Button
      type="button"
      variant="outline"
      className={className}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value)
          setCopied(true)
        } catch {
          toast.error('Kopírování se nezdařilo — označte text a zkopírujte ho ručně.')
        }
      }}
    >
      {copied ? <CheckIcon data-icon="inline-start" className="text-success" /> : <CopyIcon data-icon="inline-start" />}
      {copied ? 'Zkopírováno' : label}
    </Button>
  )
}
