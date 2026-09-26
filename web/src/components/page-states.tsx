import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { FileQuestionIcon, TriangleAlertIcon, WifiOffIcon } from 'lucide-react'
import { useEffect } from 'react'
import { errorMessage, isApiError } from '@/api/errors'
import { ButtonLink } from '@/components/button-link'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'

/** Chyba při načítání stránky (router defaultErrorComponent). */
export function PageError({ error, reset }: ErrorComponentProps) {
  const router = useRouter()
  const queryReset = useQueryErrorResetBoundary()

  useEffect(() => {
    queryReset.reset()
  }, [queryReset])

  if (isApiError(error) && error.status === 404) return <PageNotFound />

  const offline = isApiError(error) && error.isNetworkError
  return (
    <div className="mx-auto w-full max-w-xl px-4 py-12">
      <EmptyState
        icon={offline ? WifiOffIcon : TriangleAlertIcon}
        title={offline ? 'Server je nedostupný' : 'Něco se pokazilo'}
        description={errorMessage(error)}
        action={
          <Button
            onClick={() => {
              reset()
              void router.invalidate()
            }}
          >
            Zkusit znovu
          </Button>
        }
      />
    </div>
  )
}

export function PageNotFound() {
  return (
    <div className="mx-auto w-full max-w-xl px-4 py-12">
      <EmptyState
        icon={FileQuestionIcon}
        title="Stránka nenalezena"
        description="Záznam neexistuje, nebo k němu nemáte přístup."
        action={<ButtonLink to="/">Na úvod</ButtonLink>}
      />
    </div>
  )
}
