// Motiv ještě před prvním vykreslením (bez probliknutí). Viz src/components/theme.tsx.
// Samostatný soubor (ne inline skript), aby CSP mohla zakázat inline JavaScript.
try {
  var t = localStorage.getItem('nf-theme')
  var dark = t === 'dark' || (t !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches)
  if (dark) document.documentElement.classList.add('dark')
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
} catch (e) {}
