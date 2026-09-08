'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { startRegistration } from '@simplewebauthn/browser'
import type { PublicKeyCredentialCreationOptionsJSON } from '@simplewebauthn/browser'
import { Fingerprint, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useWebAuthnSupport } from '@/features/auth/hooks/useWebAuthnSupport'
import { AuthError, AuthErrorCode, type AuthErrorCodeType } from '@/lib/errors'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

interface PasskeyRegisterButtonProps {
  onSuccess?: () => void
  /**
   * The failure itself, not its sentence: registration is gated on a recent
   * sign-in, and the caller can only offer the remedy for that refusal if it
   * can read the error code off the error.
   */
  onError?: (error: unknown) => void
  className?: string
}

/**
 * Both registration steps answer a failure with success:false plus a message
 * and, for typed refusals, an error_code. The re-authentication refusal is the
 * one a caller has to tell apart, so this carries the code through instead of
 * flattening the response to its sentence.
 *
 * This is the one credential surface that does not go through `apiRequest`,
 * because it drives the WebAuthn ceremony between the two calls. Reading the
 * response is therefore its own job, including the case `apiRequest` handles
 * for everyone else: a non-JSON refusal from an edge rule or a protection page,
 * which must not surface as a parse error with no remedy.
 */
async function readRegistrationResponse(
  response: Response,
  fallback: string
): Promise<{ ok: true; body: RegistrationStepBody } | { ok: false; error: Error }> {
  let body: RegistrationStepBody | null = null
  try {
    body = (await response.json()) as RegistrationStepBody
  } catch {
    body = null
  }

  if (body?.success) return { ok: true, body }

  const message = body?.message || fallback
  const code = body?.error_code
  if (!code) return { ok: false, error: new Error(message) }
  return {
    ok: false,
    error: new AuthError(
      message,
      (code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN
    ),
  }
}

interface RegistrationStepBody {
  success?: boolean
  message?: string
  error_code?: string
  options?: PublicKeyCredentialCreationOptionsJSON
  challenge_id?: string
}

export function PasskeyRegisterButton({ onSuccess, onError, className }: PasskeyRegisterButtonProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [displayName, setDisplayName] = useState('')

  const supportsWebAuthn = useWebAuthnSupport()

  const handleRegister = async () => {
    if (!supportsWebAuthn) {
      onError?.(new Error('Your browser does not support passkeys'))
      return
    }

    setIsLoading(true)

    try {
      // Step 1: Begin registration
      const beginResponse = await fetch(`${API_BASE_URL}/auth/passkey/register/begin`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ display_name: displayName }),
      })

      const begin = await readRegistrationResponse(
        beginResponse,
        'Failed to start passkey registration'
      )
      if (!begin.ok) throw begin.error
      const beginData = begin.body
      if (!beginData.options) {
        throw new Error('Failed to start passkey registration')
      }

      // Step 2: Perform WebAuthn registration
      const credential = await startRegistration({
        optionsJSON: beginData.options,
      })

      // Step 3: Finish registration
      const finishResponse = await fetch(`${API_BASE_URL}/auth/passkey/register/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          challenge_id: beginData.challenge_id,
          display_name: displayName || 'My Passkey',
          response: credential,
        }),
      })

      const finish = await readRegistrationResponse(
        finishResponse,
        'Failed to register passkey'
      )
      if (!finish.ok) throw finish.error

      // Success
      setIsOpen(false)
      setDisplayName('')
      onSuccess?.()
    } catch (error) {
      if (error instanceof Error) {
        if (error.name === 'NotAllowedError') {
          // User cancelled
          return
        }
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'registration_failed' },
        })
        onError?.(error)
      } else {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'passkey-auth', error_type: 'registration_failed' },
        })
        onError?.(new Error('An unexpected error occurred'))
      }
    } finally {
      setIsLoading(false)
    }
  }

  if (!supportsWebAuthn) {
    return null
  }

  return (
    <Dialog open={isOpen} onOpenChange={setIsOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className={className}>
          Add passkey
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Fingerprint className="h-5 w-5" />
            Add passkey
          </DialogTitle>
          <DialogDescription>
            Passkeys let you sign in securely using your device&apos;s biometrics (Face ID, Touch ID,
            Windows Hello) or a security key.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="passkey-name">Passkey name</Label>
            <Input
              id="passkey-name"
              placeholder="e.g., My MacBook, Work laptop"
              value={displayName}
              onChange={e => setDisplayName(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Give this passkey a name to identify it later
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setIsOpen(false)} disabled={isLoading}>
            Cancel
          </Button>
          <Button onClick={handleRegister} disabled={isLoading}>
            {isLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Registering...
              </>
            ) : (
              <>
                <Fingerprint className="h-4 w-4" />
                Register passkey
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
