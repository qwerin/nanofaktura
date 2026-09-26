import { createLink } from '@tanstack/react-router'
import type { VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type ButtonAnchorProps = ComponentProps<'a'> & VariantProps<typeof buttonVariants>

function ButtonAnchor({ className, variant, size, ...props }: ButtonAnchorProps) {
  return <a data-slot="button" className={cn(buttonVariants({ variant, size }), className)} {...props} />
}

/**
 * Odkaz vypadající jako tlačítko, plně typovaný jako TanStack `<Link>`:
 *   <ButtonLink to="/a/$slug/invoices/new" params={{ slug }}>Nová faktura</ButtonLink>
 */
export const ButtonLink = createLink(ButtonAnchor)
