'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { startAuthentication } from '@simplewebauthn/browser'
import { Fingerprint, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  refreshCachesForNewSession,
  type AuthResponse,
} from '@/features/auth/hooks/useAuth'
import { useWebAuthnSupport } from '@/features/auth/hooks/useWebAuthnSupport'
import { authenticationOptionsFrom } from '@/features/auth/passkeyOptions'
import type { components } from '@/types/api'
import { API_BASE_URL } from '@/lib/api-base'
import { AuthError, AuthErrorCode } from '@/lib/errors'

interface PasskeyLoginButtonProps {
  /**
   * Called when a sign-in does not complete, except when the viewer cancels
   * the passkey prompt. A refusal the server answered carries its
   * `error_code` as the error's code; anything else is coded UNKNOWN.
   */
  onError?: (error: AuthError) => void
  className?: string
  returnTo?: string
}

export function PasskeyLoginButton({
  onError,
  className,
  returnTo = '/',
}: PasskeyLoginButtonProps) {
  const router = useRouter()
  const { setUser } = useAuthContext()
  const queryClient = useQueryClient()
  const [isLoading, setIsLoading] = useState(false)

  const supportsWebAuthn = useWebAuthnSupport()

  const handlePasskeyLogin = async () => {
    if (!supportsWebAuthn) {
      onError?.(
        new AuthError('Your browser does not support passkeys', AuthErrorCode.UNKNOWN)
      )
      return
    }

    setIsLoading(true)

    try {
      // Step 1: Begin authentication
      const beginResponse = await fetch(`${API_BASE_URL}/auth/passkey/login/begin`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({}), // Empty body for discoverable login
      })

      const beginData: components['schemas']['BeginLoginResponseBody'] =
        await beginResponse.json()

      const startFailure = 'Failed to start passkey login'
      if (!beginData.success) {
        throw new Error(beginData.message || startFailure)
      }
      const optionsJSON = authenticationOptionsFrom(beginData.options)
      if (!optionsJSON) {
        throw new Error(startFailure)
      }

      // Step 2: Perform WebAuthn authentication
      const assertion = await startAuthentication({ optionsJSON })

      // Step 3: Finish authentication
      const finishResponse = await fetch(`${API_BASE_URL}/auth/passkey/login/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          challenge_id: beginData.challenge_id,
          response: assertion,
        }),
      })

      const finishData: AuthResponse = await finishResponse.json()

      if (!finishData.success) {
        throw AuthError.fromResponse({
          message: finishData.message || 'Failed to complete passkey login',
          error_code: finishData.error_code,
          request_id: finishData.request_id,
          status: finishResponse.status,
        })
      }

      // Success - update auth context and redirect
      if (finishData.user) {
        setUser(finishData.user)
      }

      // This path establishes a session with a raw fetch rather than the
      // `useLogin` mutation, so nothing else refetches the profile the auth
      // context reads as its source of truth, or drops the caches whose
      // payload depends on the viewer's privilege tier (PSY-1857).
      //
      // Not awaited. The profile query retries a failure twice, and a 429
      // retries on the rate-limit backoff, so awaiting would hold this flow
      // behind a refetch for as long as that takes. `setUser` above already carries the
      // whole payload, so nothing here needs the profile to have landed; the
      // refetch is what lets a later change to the viewer reach the context.
      void refreshCachesForNewSession(queryClient)

      router.push(returnTo)
    } catch (error) {
      // Handle user cancellation gracefully
      if (error instanceof Error) {
        if (error.name === 'NotAllowedError') {
          // User cancelled the operation
          return
        }
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'login_failed' },
        })
        onError?.(AuthError.fromUnknown(error))
      } else {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'login_failed' },
        })
        onError?.(new AuthError('An unexpected error occurred', AuthErrorCode.UNKNOWN))
      }
    } finally {
      setIsLoading(false)
    }
  }

  // Don't render if browser doesn't support WebAuthn
  if (!supportsWebAuthn) {
    return null
  }

  return (
    <Button
      type="button"
      variant="outline"
      onClick={handlePasskeyLogin}
      disabled={isLoading}
      className={className}
    >
      {isLoading ? (
        <>
          <Loader2 className="h-4 w-4 animate-spin" />
          Authenticating...
        </>
      ) : (
        <>
          <Fingerprint className="h-4 w-4" />
          Sign in with passkey
        </>
      )}
    </Button>
  )
}
