'use client'

import { useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import Link from 'next/link'
import { startRegistration } from '@simplewebauthn/browser'
import { AlertCircle, Fingerprint, Loader2, Mail } from 'lucide-react'
import { z } from 'zod'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  refreshCachesForNewSession,
  type AuthResponse,
} from '@/features/auth/hooks/useAuth'
import { useWebAuthnSupport } from '@/features/auth/hooks/useWebAuthnSupport'
import {
  ConsentVersionsChangedError,
  ConsentVersionsUnavailableError,
  isConsentRefusal,
  useConsentVersions,
  type ConsentVersions,
} from '@/features/auth/hooks/useLegalVersions'
import { registrationOptionsFrom } from '@/features/auth/passkeyOptions'
import type { components } from '@/types/api'
import { MIN_SIGNUP_AGE } from '@/lib/legal'
import { API_BASE_URL } from '@/lib/api-base'
import {
  CONSENT_VERSIONS_UNAVAILABLE,
  ConsentRetryNotice,
  ConsentVersionsChangedNotice,
} from './consent-versions-notice'
import type { SignupHandoff } from './signup-handoff'
import type { SignupRequestDeadline } from './signup-request-deadline'

const START_FAILURE = 'Failed to start passkey signup'
const FINISH_FAILURE = 'Failed to complete passkey signup'

type BeginSignupResponse = components['schemas']['BeginSignupResponseBody']

interface PasskeySignupButtonProps {
  /**
   * Receives an error raised while the dialog is closed. One raised while it
   * is open shows inside the dialog instead and never reaches this.
   */
  onError?: (error: string) => void
  /**
   * Reports this sign-up's claim on the page: `pending` before the request
   * that creates the session goes out, `complete` at its first step once the
   * session is set (this button's dialog closes at the same time), and `null`
   * when the attempt fails after claiming. The steps after the first, and the
   * navigation after them, belong to the page.
   */
  onHandoffChange: (handoff: SignupHandoff | null) => void
  /**
   * Bounds the begin and finish requests. A request that times out fails with
   * the generic message for its step. Owned by the page that owns the handoff.
   */
  requestDeadline: SignupRequestDeadline
  className?: string
  minAge?: number
  /**
   * The form's latest read found consent versions other than this build's.
   * While it holds, the dialog says so and sends nothing.
   */
  versionsChanged?: boolean
}

export function PasskeySignupButton({
  onError,
  onHandoffChange,
  requestDeadline,
  className,
  minAge = MIN_SIGNUP_AGE,
  versionsChanged = false,
}: PasskeySignupButtonProps) {
  const { setUser } = useAuthContext()
  const queryClient = useQueryClient()
  const consentVersions = useConsentVersions()
  const [isLoading, setIsLoading] = useState(false)
  const [isRetryingConsent, setIsRetryingConsent] = useState(false)
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  // Whether the dialog is open now, read by a request that settles after the
  // render it started from.
  const dialogOpenRef = useRef(false)
  const [dialogError, setDialogError] = useState<string | null>(null)
  // The dialog opens without an error: one shown before it was closed belongs
  // to an attempt that is over.
  const setDialogOpen = (open: boolean) => {
    dialogOpenRef.current = open
    setIsDialogOpen(open)
    if (open) setDialogError(null)
  }
  const emailInputRef = useRef<HTMLInputElement>(null)
  const [email, setEmail] = useState('')
  const [emailError, setEmailError] = useState<string | null>(null)
  const [termsAccepted, setTermsAccepted] = useState(false)
  const [termsError, setTermsError] = useState<string | null>(null)
  const [ageConfirmed, setAgeConfirmed] = useState(false)
  const [ageError, setAgeError] = useState<string | null>(null)

  const supportsWebAuthn = useWebAuthnSupport()

  // The page's alert sits behind the open dialog's overlay, so an error raised
  // while the dialog is open shows in the dialog's own alert, with the
  // controls enabled again and focus back on Email. The alert announces
  // itself, so it does not take focus.
  const reportError = (message: string) => {
    if (!dialogOpenRef.current) {
      onError?.(message)
      return
    }
    flushSync(() => {
      setDialogError(message)
      setIsLoading(false)
    })
    emailInputRef.current?.focus()
  }

  const handlePasskeySignup = async () => {
    setDialogError(null)

    // Validate email
    const emailSchema = z.string().email('Please enter a valid email address')
    const result = emailSchema.safeParse(email)
    if (!result.success) {
      setEmailError(result.error.issues[0].message)
      return
    }
    setEmailError(null)

    // Validate terms acceptance
    if (!termsAccepted) {
      setTermsError('You must agree to the Terms of Service and Privacy Policy')
      return
    }
    setTermsError(null)

    // Validate age confirmation (PSY-1023)
    if (!ageConfirmed) {
      setAgeError(`You must confirm that you are at least ${minAge} years old`)
      return
    }
    setAgeError(null)

    if (!supportsWebAuthn) {
      reportError('Your browser does not support passkeys')
      return
    }

    setIsLoading(true)
    let claimed = false
    // The signal of the request most recently put under the deadline. A
    // failure while it is aborted is that request timing out.
    let deadlineSignal: AbortSignal | null = null

    // Step 1: Begin signup, under the deadline. The versions are the ones the
    // backend publishes, never this build's.
    const beginSignup = async (
      versions: ConsentVersions,
      signal: AbortSignal
    ): Promise<BeginSignupResponse> => {
      const beginResponse = await fetch(`${API_BASE_URL}/auth/passkey/signup/begin`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        signal,
        body: JSON.stringify({
          email,
          terms_accepted: true,
          terms_version: versions.termsVersion,
          privacy_version: versions.privacyVersion,
          age_confirmed: true,
          min_age_attested: minAge,
        }),
      })
      const beginData: BeginSignupResponse = await beginResponse.json()
      // The ceremony after it waits on the person, not the network.
      requestDeadline.disarm()
      return beginData
    }

    try {
      let versions = await consentVersions.read()
      deadlineSignal = requestDeadline.arm()
      let beginData = await beginSignup(versions, deadlineSignal)
      // Refused with versions the backend published: the read was older than
      // the backend's answer. Read again past every cache and begin once more;
      // a second refusal is shown as it is.
      if (!beginData.success && isConsentRefusal(beginData.error_code)) {
        setIsRetryingConsent(true)
        versions = await consentVersions.refresh()
        deadlineSignal = requestDeadline.arm()
        beginData = await beginSignup(versions, deadlineSignal)
        setIsRetryingConsent(false)
      }

      if (!beginData.success) {
        throw new Error(beginData.message || START_FAILURE)
      }
      const optionsJSON = registrationOptionsFrom(beginData.options)
      if (!optionsJSON) {
        throw new Error(START_FAILURE)
      }

      // Step 2: Perform WebAuthn registration
      const credential = await startRegistration({ optionsJSON })

      // Step 3: Finish signup. Claimed before the request goes out; see the
      // SignupHandoff doc comment for the redirect race this closes.
      onHandoffChange({ status: 'pending', email })
      claimed = true
      deadlineSignal = requestDeadline.arm()
      const finishResponse = await fetch(`${API_BASE_URL}/auth/passkey/signup/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        signal: deadlineSignal,
        body: JSON.stringify({
          challenge_id: beginData.challenge_id,
          response: credential,
          terms_accepted: true,
          terms_version: versions.termsVersion,
          privacy_version: versions.privacyVersion,
          age_confirmed: true,
          min_age_attested: minAge,
        }),
      })

      const finishData: AuthResponse = await finishResponse.json()

      if (!finishData.success) {
        throw new Error(finishData.message || FINISH_FAILURE)
      }

      if (!finishData.user) {
        // No session to hand off to. The catch releases the claim and says so.
        throw new Error(FINISH_FAILURE)
      }

      setUser(finishData.user)
      setDialogOpen(false)
      onHandoffChange({
        status: 'complete',
        email: finishData.user.email || email,
        method: 'passkey',
        step: 'backup-prompt',
      })

      // This path establishes a session with a raw fetch rather than the
      // `useRegister` mutation, so nothing else refetches the profile the auth
      // context reads as its source of truth, or drops the caches whose
      // payload depends on the viewer's privilege tier (PSY-1857).
      //
      // Not awaited. The profile query retries a failure twice, and a 429
      // retries on the rate-limit backoff, so awaiting would hold this flow
      // behind a refetch for as long as that takes. `setUser` above already carries the
      // whole payload, so nothing here needs the profile to have landed; the
      // refetch is what lets a later change to the viewer reach the context.
      void refreshCachesForNewSession(queryClient)
    } catch (error) {
      if (claimed) {
        onHandoffChange(null)
      }

      // The begin and finish requests call fetch directly, not apiRequest, so
      // this component maps a request that timed out to its step's message
      // itself.
      if (deadlineSignal?.aborted) {
        reportError(claimed ? FINISH_FAILURE : START_FAILURE)
        return
      }

      if (error instanceof ConsentVersionsUnavailableError) {
        reportError(CONSENT_VERSIONS_UNAVAILABLE)
        return
      }
      // Shown in the dialog from versionsChanged, once the page has the read.
      if (error instanceof ConsentVersionsChangedError) {
        return
      }

      // Handle user cancellation gracefully
      if (error instanceof Error) {
        if (error.name === 'NotAllowedError') {
          // User cancelled the operation
          return
        }
        reportError(error.message)
      } else {
        reportError('An unexpected error occurred')
      }
    } finally {
      requestDeadline.disarm()
      setIsRetryingConsent(false)
      setIsLoading(false)
    }
  }

  // Don't render if browser doesn't support WebAuthn
  if (!supportsWebAuthn) {
    return null
  }

  return (
    <Dialog open={isDialogOpen} onOpenChange={setDialogOpen}>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={className}
          disabled={versionsChanged}
        >
          <Fingerprint className="h-4 w-4" />
          Sign up with passkey
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create account with passkey</DialogTitle>
          <DialogDescription>
            Enter your email address to create an account secured with your device&apos;s passkey.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          {dialogError && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{dialogError}</AlertDescription>
            </Alert>
          )}
          <div className="space-y-2">
            <Label htmlFor="passkey-email">Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                ref={emailInputRef}
                id="passkey-email"
                type="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value)
                  setEmailError(null)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    handlePasskeySignup()
                  }
                }}
                className="pl-10"
                disabled={isLoading}
              />
            </div>
            {emailError && (
              <p role="alert" className="text-sm text-destructive">{emailError}</p>
            )}
          </div>
          <div className="space-y-2">
            <div className="flex items-start space-x-3">
              <Checkbox
                id="passkey-terms"
                checked={termsAccepted}
                onCheckedChange={(checked) => {
                  setTermsAccepted(checked === true)
                  setTermsError(null)
                }}
                className="mt-0.5"
              />
              <Label
                htmlFor="passkey-terms"
                className="text-sm font-normal leading-relaxed cursor-pointer"
              >
                I agree to the{' '}
                <Link
                  href="/terms"
                  target="_blank"
                  className="font-medium underline underline-offset-4 hover:text-primary"
                >
                  Terms of Service
                </Link>{' '}
                and{' '}
                <Link
                  href="/privacy"
                  target="_blank"
                  className="font-medium underline underline-offset-4 hover:text-primary"
                >
                  Privacy Policy
                </Link>
              </Label>
            </div>
            {termsError && (
              <p role="alert" className="text-sm text-destructive">{termsError}</p>
            )}
          </div>
          <div className="space-y-2">
            <div className="flex items-start space-x-3">
              <Checkbox
                id="passkey-age-confirmation"
                checked={ageConfirmed}
                onCheckedChange={(checked) => {
                  setAgeConfirmed(checked === true)
                  setAgeError(null)
                }}
                className="mt-0.5"
              />
              <Label
                htmlFor="passkey-age-confirmation"
                className="text-sm font-normal leading-relaxed cursor-pointer"
              >
                I confirm that I am at least {minAge} years old
              </Label>
            </div>
            {ageError && (
              <p role="alert" className="text-sm text-destructive">{ageError}</p>
            )}
          </div>
          {versionsChanged ? (
            <ConsentVersionsChangedNotice />
          ) : (
            isRetryingConsent && <ConsentRetryNotice />
          )}
          <Button
            type="button"
            onClick={handlePasskeySignup}
            disabled={
              isLoading || versionsChanged || !email || !termsAccepted || !ageConfirmed
            }
            className="w-full"
          >
            {isLoading ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              <>
                <Fingerprint className="h-4 w-4" />
                Continue with passkey
              </>
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
