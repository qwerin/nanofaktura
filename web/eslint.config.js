import js from '@eslint/js'
import pluginQuery from '@tanstack/eslint-plugin-query'
import pluginRouter from '@tanstack/eslint-plugin-router'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import { defineConfig, globalIgnores } from 'eslint/config'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default defineConfig([
  globalIgnores(['dist', 'dev-dist', 'public', 'src/routeTree.gen.ts', 'src/api/schema.gen.ts']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat['recommended-latest'],
      reactRefresh.configs.vite,
      pluginQuery.configs['flat/recommended'],
      pluginRouter.configs['flat/recommended'],
    ],
    languageOptions: {
      ecmaVersion: 2023,
      globals: globals.browser,
    },
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'react-refresh/only-export-components': ['error', { allowConstantExport: true, extraHOCs: ['createLink'] }],
    },
  },
  {
    // shadcn komponenty (generované CLI), route soubory (exportují `Route`) a testy — HMR pravidlo zde neplatí.
    files: ['src/components/ui/**', 'src/routes/**', 'src/test/**', 'src/**/*.test.tsx'],
    rules: { 'react-refresh/only-export-components': 'off' },
  },
])
