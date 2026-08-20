'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { buildAuthHref } from '@/lib/auth-href'

interface SignInPromptProps {
  /** Copy that follows the link, e.g. "to join the discussion." */
  children: React.ReactNode
  /**
   * Fragment appended to the return destination so the reader lands back on
   * the section they were trying to use rather than the top of a long page.
   * Pass the bare id, without the leading `#`.
   */
  returnToHash?: string
  testId?: string
}

/**
 * Inline "Sign in to <do the thing>" prompt for logged-out readers.
 *
 * The auth form lives at `/auth`; there is no `/login` route, which is what
 * this component exists to stop anyone from assuming again.
 *
 * The return destination is the current pathname, and NOT the query string.
 * Other sign-in entry points do include it (`VenuePanel`, `UserFollowButton`),
 * but both of those build their href inside a click handler, where
 * `window.location.search` is safe to read. This one builds an href during
 * render, including on the server, where the query string is not available
 * and the fragment is never sent at all — reading either would make the
 * server and client markup disagree. `useSearchParams` would close half the
 * gap but forces a Suspense boundary on every consumer. None of the surfaces
 * using this prompt today carry state in the query string, so the pathname is
 * the whole destination; a surface that does carry such state needs a
 * click-time href instead of this component.
 */
export function SignInPrompt({
  children,
  returnToHash,
  testId,
}: SignInPromptProps) {
  const pathname = usePathname()
  const returnTo = returnToHash ? `${pathname}#${returnToHash}` : pathname

  return (
    <p className="text-sm text-muted-foreground mb-6" data-testid={testId}>
      <Link
        href={buildAuthHref(returnTo)}
        className="text-primary hover:underline"
      >
        Sign in
      </Link>{' '}
      {children}
    </p>
  )
}
