import { useEffect, useState } from 'react'

/** Hodnota zpožděná o `delay` ms (hledání při psaní bez dotazu na každý znak). */
export function useDebouncedValue<T>(value: T, delay = 250): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}
