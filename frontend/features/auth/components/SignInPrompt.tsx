'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'

interface SignInPromptProps {
  /** Copy that follows the link, e.g. "to join the discussion." */
  children: React.ReactNode
  /**
   * Optional fragment appended to the return destination so the reader
   * lands back on the section they were trying to use, not the top of
   * the page. Pass the bare id, without the leading `#`.
   */
  returnToHash?: string
  className?: string
  testId?: string
}

/**
 * Inline "Sign in to <do the thing>" prompt for logged-out readers.
 *
 * The auth form lives at `/auth`; there is no `/login` route. Keeping the
 * href in one component means a future move of the auth route is a
 * single edit rather than a hunt through every gated surface.
 *
 * The return destination is the current pathname. Query params are
 * deliberately omitted: reading them needs `useSearchParams`, which
 * forces a Suspense boundary on every consumer, and no gated surface
 * carries meaningful state in the query string today.
 */
export function SignInPrompt({
  children,
  returnToHash,
  className,
  testId,
}: SignInPromptProps) {
  const pathname = usePathname()
  const returnTo = returnToHash ? `${pathname}#${returnToHash}` : pathname
  const href = `/auth?returnTo=${encodeURIComponent(returnTo)}`

  return (
    <p className={className} data-testid={testId}>
      <Link href={href} className="text-primary hover:underline">
        Sign in
      </Link>{' '}
      {children}
    </p>
  )
}
