import { ChevronDownIcon, ShieldCheckIcon } from 'lucide-react'
import { useState } from 'react'
import { CopyIconButton } from '@/components/subject/copy-icon-button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

const nodeSnippet = `import crypto from 'node:crypto'

// Express: app.post('/hooks/nanofaktura', express.raw({ type: 'application/json' }), handler)
export function verifySignature(rawBody, header, secret) {
  const expected =
    'sha256=' + crypto.createHmac('sha256', secret).update(rawBody).digest('hex')
  const a = Buffer.from(expected)
  const b = Buffer.from(header ?? '')
  return a.length === b.length && crypto.timingSafeEqual(a, b)
}

// verifySignature(req.body, req.get('X-NanoFaktura-Signature'), process.env.WEBHOOK_SECRET)`

const goSnippet = `import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// body = přesně přijaté bajty (io.ReadAll(r.Body)), header = r.Header.Get("X-NanoFaktura-Signature")
func verifySignature(body []byte, header, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(header))
}`

const payloadExample = `{
  "id": 123,
  "event": "invoice.paid",
  "created_at": "2026-09-29T10:15:00Z",
  "account": "moje-firma",
  "subject": { "type": "invoice", "id": 42 },
  "text": "Faktura 2026-0042 byla uhrazena",
  "data": { … }
}`

/** Nápověda: formát požadavku a ověření HMAC podpisu (Node + Go). */
export function SignatureDocs() {
  const [open, setOpen] = useState(false)
  return (
    <section className="rounded-xl border bg-card">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex min-h-12 w-full items-center gap-2 rounded-xl px-4 py-3 text-left focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        <ShieldCheckIcon className="size-4 text-muted-foreground" />
        <h2 className="flex-1 text-base font-semibold tracking-tight">Jak ověřit podpis</h2>
        <ChevronDownIcon className={cn('size-4 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="flex flex-col gap-4 px-4 pb-4 text-sm">
          <p className="text-muted-foreground">
            Každý požadavek je <code className="font-mono text-xs">POST</code> s JSONem a hlavičkami{' '}
            <code className="font-mono text-xs">X-NanoFaktura-Event</code>, <code className="font-mono text-xs">X-NanoFaktura-Delivery</code> a{' '}
            <code className="font-mono text-xs">X-NanoFaktura-Signature: sha256=&lt;hex&gt;</code> — HMAC-SHA256 z nezměněného těla požadavku,
            klíčem je tajemství webhooku. Odpovězte stavem 2xx do 10 s; jinak to zkusíme znovu za 1 min, 5 min, 30 min, 2 h a 12 h.
            Přesměrování (3xx) se nepovažuje za úspěch.
          </p>
          <Snippet code={payloadExample} label="Tělo požadavku" />
          <Tabs defaultValue="node">
            <TabsList>
              <TabsTrigger value="node" className="px-3">
                Node.js
              </TabsTrigger>
              <TabsTrigger value="go" className="px-3">
                Go
              </TabsTrigger>
            </TabsList>
            <TabsContent value="node">
              <Snippet code={nodeSnippet} />
            </TabsContent>
            <TabsContent value="go">
              <Snippet code={goSnippet} />
            </TabsContent>
          </Tabs>
        </div>
      )}
    </section>
  )
}

function Snippet({ code, label }: { code: string; label?: string }) {
  return (
    <div className="flex flex-col gap-1">
      {label && <span className="text-xs font-medium text-muted-foreground">{label}</span>}
      <div className="relative">
        <pre className="overflow-x-auto rounded-lg border bg-muted px-3 py-3 pr-12 font-mono text-xs leading-relaxed">{code}</pre>
        <CopyIconButton value={code} label="Zkopírovat kód" className="absolute top-1 right-1" />
      </div>
    </div>
  )
}
