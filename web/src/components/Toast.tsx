import { useEffect, useState, useCallback } from 'react'

export type ToastVariant = 'success' | 'error'

export interface ToastMessage {
  id: number
  message: string
  variant: ToastVariant
}

let nextId = 0
type Listener = (toasts: ToastMessage[]) => void
let toasts: ToastMessage[] = []
const listeners = new Set<Listener>()

function notify() {
  listeners.forEach(l => l([...toasts]))
}

export function toast(message: string, variant: ToastVariant = 'success') {
  const id = ++nextId
  toasts = [...toasts, { id, message, variant }]
  notify()
  setTimeout(() => {
    toasts = toasts.filter(t => t.id !== id)
    notify()
  }, 4000)
}

export function useToasts() {
  const [items, setItems] = useState<ToastMessage[]>([])
  useEffect(() => {
    listeners.add(setItems)
    return () => { listeners.delete(setItems) }
  }, [])
  const dismiss = useCallback((id: number) => {
    toasts = toasts.filter(t => t.id !== id)
    notify()
  }, [])
  return { items, dismiss }
}

export function ToastContainer() {
  const { items, dismiss } = useToasts()
  if (items.length === 0) return null
  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col gap-2 pointer-events-none">
      {items.map(t => (
        <div
          key={t.id}
          onClick={() => dismiss(t.id)}
          className={[
            'pointer-events-auto flex items-center gap-3 rounded-lg px-4 py-3 text-sm font-medium shadow-lg',
            'animate-in slide-in-from-bottom-2 fade-in duration-200 cursor-pointer',
            t.variant === 'success'
              ? 'bg-green-600 text-white'
              : 'bg-red-600 text-white',
          ].join(' ')}
        >
          <span className="text-base">{t.variant === 'success' ? '✓' : '✕'}</span>
          {t.message}
        </div>
      ))}
    </div>
  )
}
