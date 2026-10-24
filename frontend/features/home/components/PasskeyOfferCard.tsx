'use client'

import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/button'
// Concrete module paths, not the barrels: this component is reachable from the
// home route. See features/sharedChunkBarrelGuard.test.ts.
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'
import { CredentialErrorMessage } from '@/components/shared/CredentialErrorMessage'
import { PasskeyRegisterButton } from '@/features/auth/components/PasskeyRegisterButton'
import { useProfile, type UserProfile } from '@/features/auth/hooks/useAuth'
import { useWebAuthnSupport } from '@/features/auth/hooks/useWebAuthnSupport'
import { useAuthContext } from '@/lib/context/AuthContext'
import { refetchSignedInProfile } from '@/lib/queryClient'
import { usePasskeyOfferDismissal } from '../hooks/usePasskeyOfferDismissal'

const REGISTER_FAILED = 'Your passkey was not added. Please try again.'

/**
 * Whether the profile describes an account created through a provider that
 * holds neither a password nor a passkey: the only account the offer is for.
 *
 * Requires an explicit `false` and `0`. The profile omits both when it could
 * not read them, and an unknown answer is no reason to offer.
 */
export function profileOffersPasskey(profile: UserProfile | undefined): boolean {
  return (
    !!profile?.success &&
    (profile.user?.oauth_accounts?.length ?? 0) > 0 &&
    profile.has_password === false &&
    profile.passkey_count === 0
  )
}

/**
 * The signed-in home's offer to add a passkey, between the toolbar and the
 * first section. A card in the page, never a modal: the viewer is not stopped
 * on the way to their home.
 *
 * It shows on every home visit while the account qualifies, until "Not now"
 * dismisses it for this browser for good or the account gains a password or a
 * passkey. A successful registration refetches the profile, whose passkey
 * count then retires the card.
 */
export function PasskeyOfferCard() {
  const { user } = useAuthContext()
  const { data: profile } = useProfile()
  const supportsWebAuthn = useWebAuthnSupport()
  const { dismissed, dismiss } = usePasskeyOfferDismissal(user?.id ?? '')
  const queryClient = useQueryClient()
  // The error itself, not its sentence: only the error carries the code that
  // tells a refusal for want of a recent sign-in from an ordinary failure.
  const [registerError, setRegisterError] = useState<unknown>(null)

  if (!user?.id || dismissed || !supportsWebAuthn || !profileOffersPasskey(profile)) {
    return null
  }

  return (
    <section
      aria-labelledby="passkey-offer-title"
      className="mb-10 mt-10 flex flex-col gap-3.5 rounded-lg border border-border bg-card px-4 py-4 text-card-foreground sm:px-5"
    >
      <div className="flex flex-col gap-3.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
        <div className="flex min-w-0 items-start gap-3.5">
          <div
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary"
            aria-hidden
          >
            <KeyRound className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <h2
              id="passkey-offer-title"
              className="text-[15px] font-semibold leading-5 text-foreground"
            >
              Add a passkey for faster sign-in
            </h2>
            <p className="mt-1 text-sm leading-[19px] text-muted-foreground">
              Next time, sign in with your fingerprint, face or screen lock, no
              trip to Google.
            </p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2 sm:flex sm:shrink-0 sm:items-center">
          <PasskeyRegisterButton
            variant="default"
            className="w-full sm:w-auto"
            onSuccess={() => {
              setRegisterError(null)
              void refetchSignedInProfile(queryClient)
            }}
            onError={error => setRegisterError(error)}
          />
          <Button
            variant="ghost"
            size="sm"
            className="w-full sm:w-auto"
            onClick={dismiss}
          >
            Not now
          </Button>
        </div>
      </div>
      {registerError !== null && (
        <InlineErrorBanner>
          <CredentialErrorMessage
            error={registerError}
            fallback={
              registerError instanceof Error && registerError.message
                ? registerError.message
                : REGISTER_FAILED
            }
          />
        </InlineErrorBanner>
      )}
    </section>
  )
}
