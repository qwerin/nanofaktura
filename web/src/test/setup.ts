// Setup komponentových testů (jsdom): jest-dom matchery, msw, polyfilly prohlížečových API.
import '@testing-library/jest-dom/vitest'
import '@/lib/zod-config'
import { cleanup } from '@testing-library/react'
import { afterAll, afterEach, beforeEach, vi } from 'vitest'
import { resetState, server } from './server'
import { applyViewport, resetViewport, viewport } from './viewport'

// msw musí nahradit fetch dřív, než se naimportuje api klient (openapi-fetch si fetch drží).
server.listen({ onUnhandledRequest: 'warn' })

function matchMedia(query: string): MediaQueryList {
  const max = /max-width:\s*(\d+)px/.exec(query)
  const min = /min-width:\s*(\d+)px/.exec(query)
  let matches = false
  if (max) matches = viewport.width <= Number(max[1])
  else if (min) matches = viewport.width >= Number(min[1])
  else if (query.includes('prefers-color-scheme: dark')) matches = viewport.dark
  else if (query.includes('pointer: coarse')) matches = viewport.width < 768
  return {
    matches,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  }
}
Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(matchMedia) })

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
class IntersectionObserverStub {
  root = null
  rootMargin = ''
  thresholds = []
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver
globalThis.IntersectionObserver ??= IntersectionObserverStub as unknown as typeof IntersectionObserver
window.scrollTo = () => {}
Element.prototype.scrollIntoView ??= function () {}
Element.prototype.hasPointerCapture ??= () => false
Element.prototype.setPointerCapture ??= () => {}
Element.prototype.releasePointerCapture ??= () => {}
if (!window.PointerEvent) {
  // Base UI používá PointerEvent; jsdom ho nemá.
  class PointerEventStub extends MouseEvent {
    pointerId = 1
    pointerType = 'mouse'
  }
  window.PointerEvent = PointerEventStub as unknown as typeof PointerEvent
}

beforeEach(() => {
  applyViewport()
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  server.resetHandlers()
  resetState()
  resetViewport()
})

afterAll(() => server.close())
