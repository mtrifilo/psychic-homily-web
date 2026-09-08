'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { isReauthRequired, REAUTH_REQUIRED_MESSAGE } from '@/lib/errors'
import { buildReauthHref, FALLBACK_RETURN_TO } from '@/lib/auth-href'

export interface CredentialErrorMessageProps {
  /** The failed mutation's error. */
  error: unknown
  /** Copy for every failure that is not the re-authentication refusal. */
  fallback: string
}

/**
 * What a surface renders when a credential operation fails.
 *
 * Named for credentials rather than for mints because the passkey card routes
 * its delete failures through it too: adding is what the gate refuses, but a
 * card with one alert region has one place to render every failure.
 *
 * The re-authentication refusal is the one failure whose remedy is not "try
 * again": the reader is still signed in, so nothing else on the page would let
 * them prove the account, and without this control the copy names something
 * they cannot do. The link carries the reason the auth page needs to show them
 * the form rather than bounce them straight back.
 *
 * The destination is the pathname alone, which is the render-time grade
 * currentLocationReturnTo documents: a query string read during render is not
 * reliable. The settings surfaces come back to /profile rather than to the
 * Settings tab, which is one click, not a dead end.
 */
export function CredentialErrorMessage({ error, fallback }: CredentialErrorMessageProps) {
  const pathname = usePathname()

  if (!isReauthRequired(error)) return <>{fallback}</>

  return (
    <>
      {REAUTH_REQUIRED_MESSAGE}{' '}
      <Link
        href={buildReauthHref(pathname || FALLBACK_RETURN_TO)}
        className="font-medium underline underline-offset-2"
      >
        Sign in again
      </Link>
    </>
  )
}
