'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { startAuthentication, browserSupportsWebAuthn } from '@simplewebauthn/browser'
import { Fingerprint, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

interface PasskeyLoginButtonProps {
  onError?: (error: string) => void
  className?: string
}

export function PasskeyLoginButton({ onError, className }: PasskeyLoginButtonProps) {
  const router = useRouter()
  const { setUser } = useAuthContext()
  const [isLoading, setIsLoading] = useState(false)

  // Check if browser supports WebAuthn
  const supportsWebAuthn = browserSupportsWebAuthn()

  const handlePasskeyLogin = async () => {
    if (!supportsWebAuthn) {
      onError?.('Your browser does not support passkeys')
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

      const beginData = await beginResponse.json()

      if (!beginData.success) {
        throw new Error(beginData.message || 'Failed to start passkey login')
      }

      // Step 2: Perform WebAuthn authentication
      const assertion = await startAuthentication({
        optionsJSON: beginData.options,
      })

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

      const finishData = await finishResponse.json()

      if (!finishData.success) {
        throw new Error(finishData.message || 'Failed to complete passkey login')
      }

      // Success - update auth context and redirect
      if (finishData.user) {
        setUser({
          id: finishData.user.id,
          email: finishData.user.email,
          first_name: finishData.user.first_name,
          last_name: finishData.user.last_name,
          email_verified: finishData.user.email_verified,
        })
      }

      router.push('/')
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
        onError?.(error.message)
      } else {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'login_failed' },
        })
        onError?.('An unexpected error occurred')
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

export { browserSupportsWebAuthn }
