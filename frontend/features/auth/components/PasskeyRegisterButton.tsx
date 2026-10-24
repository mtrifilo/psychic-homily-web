'use client'

import { useState, type ComponentProps } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import * as Sentry from '@sentry/nextjs'
import { startRegistration } from '@simplewebauthn/browser'
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
import { registrationOptionsFrom } from '@/features/auth/passkeyOptions'
import { AuthError, type AuthErrorCodeType } from '@/lib/errors'
import type { components } from '@/types/api'
import { API_BASE_URL } from '@/lib/api-base'
import { refetchSignedInProfile } from '@/lib/queryClient'

type BeginRegisterResponse = components['schemas']['BeginRegisterResponseBody']
type FinishRegisterResponse = components['schemas']['FinishRegisterResponseBody']

interface PasskeyRegisterButtonProps {
  onSuccess?: () => void
  /**
   * The failure itself, not its sentence: registration is gated on a recent
   * sign-in, and the caller can only offer the remedy for that refusal if it
   * can read the error code off the error.
   */
  onError?: (error: unknown) => void
  className?: string
  /** The trigger's button variant. The dialog it opens is the same for all. */
  variant?: ComponentProps<typeof Button>['variant']
}

/**
 * Both registration steps answer a failure with success:false plus a message
 * and, for typed refusals, an error_code. The re-authentication refusal is the
 * one a caller has to tell apart, so this carries the code through instead of
 * flattening the response to its sentence.
 *
 * Both registration requests call fetch directly, not `apiRequest`, because
 * the WebAuthn ceremony runs between them. Reading the response is therefore
 * its own job, including the case `apiRequest` handles for its callers: a
 * non-JSON refusal from an edge rule or a protection page, which must not
 * surface as a parse error with no remedy.
 */
async function readRegistrationResponse<
  Body extends BeginRegisterResponse | FinishRegisterResponse,
>(
  response: Response,
  fallback: string
): Promise<{ ok: true; body: Body } | { ok: false; error: Error }> {
  let body: Body | null = null
  try {
    body = (await response.json()) as Body
  } catch {
    body = null
  }

  if (body?.success) return { ok: true, body }

  const message = body?.message || fallback
  const code = body?.error_code
  if (!code) return { ok: false, error: new Error(message) }
  return {
    ok: false,
    error: new AuthError(message, code as AuthErrorCodeType),
  }
}

export function PasskeyRegisterButton({
  onSuccess,
  onError,
  className,
  variant = 'outline',
}: PasskeyRegisterButtonProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [displayName, setDisplayName] = useState('')

  const supportsWebAuthn = useWebAuthnSupport()
  const queryClient = useQueryClient()

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

      const begin = await readRegistrationResponse<BeginRegisterResponse>(
        beginResponse,
        'Failed to start passkey registration'
      )
      if (!begin.ok) throw begin.error
      const beginData = begin.body
      const optionsJSON = registrationOptionsFrom(beginData.options)
      if (!optionsJSON) {
        throw new Error('Failed to start passkey registration')
      }

      // Step 2: Perform WebAuthn registration
      const credential = await startRegistration({ optionsJSON })

      // Step 3: Finish registration
      const finishResponse = await fetch(`${API_BASE_URL}/auth/passkey/register/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          challenge_id: beginData.challenge_id,
          display_name: displayName,
          response: credential,
        }),
      })

      const finish = await readRegistrationResponse<FinishRegisterResponse>(
        finishResponse,
        'Failed to register passkey'
      )
      if (!finish.ok) throw finish.error

      // Success. The profile carries the account's passkey count, so every
      // surface that reads it sees the new passkey.
      setIsOpen(false)
      setDisplayName('')
      void refetchSignedInProfile(queryClient)
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
        <Button variant={variant} size="sm" className={className}>
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
              maxLength={100}
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
