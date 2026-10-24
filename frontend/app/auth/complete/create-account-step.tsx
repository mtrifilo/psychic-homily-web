'use client'

import { useId, useRef, useState, type ReactNode } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'
import {
  useFinishOAuthSignup,
  usePendingOAuthSignup,
  type PendingOAuthSignup,
} from '@/features/auth/hooks/useOAuthSignup'
import {
  ConsentVersionsChangedError,
  useConsentVersions,
  useLegalVersions,
} from '@/features/auth/hooks/useLegalVersions'
import { useAuthContext } from '@/lib/context/AuthContext'
import { AuthErrorCode, isAuthError } from '@/lib/errors'
import { FALLBACK_RETURN_TO } from '@/lib/auth-href'
import { cn } from '@/lib/utils'
import {
  CONSENT_VERSIONS_UNAVAILABLE,
  ConsentVersionsChangedNotice,
} from '@/app/auth/_components/consent-versions-notice'
import {
  GoogleIcon,
  GoogleOAuthButton,
} from '@/app/auth/_components/google-oauth-button'
import { googleStartUrl } from '@/app/auth/_components/google-start-url'
import { AgeRefusedCard } from '@/app/auth/_components/age-refused-card'
import {
  DateOfBirthField,
  EMPTY_DATE_OF_BIRTH,
  dateOfBirthFromParts,
  firstInvalidDateOfBirthPartId,
} from '@/app/auth/_components/date-of-birth-field'
import { StepCard, StepHeading, StepPage } from '@/app/auth/_components/step-card'
import { focusControl } from '@/app/auth/_components/form-error-focus'
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'

export const TERMS_REQUIRED_MESSAGE =
  'Agree to the Terms of Service to create your account.'

const LOAD_FAILED_MESSAGE =
  'Service temporarily unavailable. Please try again.'

/**
 * The "Create your account" step a new Google identity finishes on. What it
 * shows is derived from the pending sign-up the backend holds for this browser
 * and from the finish's answer: the form, the expired step, or the under-age
 * refusal.
 */
export function CreateAccountStep() {
  const pending = usePendingOAuthSignup()
  const finish = useFinishOAuthSignup()
  const finishCode = isAuthError(finish.error) ? finish.error.code : null
  // A card that replaces a submitted form takes focus, so the outcome is
  // announced rather than left behind a focus that fell to the page.
  const replacedForm = finish.isError
  const returnTo =
    pending.data?.status === 'pending'
      ? sanitizeReturnTo(pending.data.return_to ?? FALLBACK_RETURN_TO)
      : FALLBACK_RETURN_TO

  let content: ReactNode
  if (pending.isPending) {
    content = (
      <div className="flex justify-center py-12" role="status" aria-label="Loading">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  } else if (pending.isError) {
    content = (
      <InlineErrorBanner variant="queryFallback">
        {LOAD_FAILED_MESSAGE}
      </InlineErrorBanner>
    )
  } else if (
    pending.data.status === 'age_refused' ||
    finishCode === AuthErrorCode.AGE_CONFIRMATION_REQUIRED
  ) {
    content = <AgeRefusedCard focusHeading={replacedForm} />
  } else if (
    pending.data.status === 'expired' ||
    finishCode === AuthErrorCode.OAUTH_SIGNUP_EXPIRED
  ) {
    content = <ExpiredCard focusHeading={replacedForm} returnTo={returnTo} />
  } else {
    content = <CreateAccountForm pending={pending.data} finish={finish} />
  }

  return <StepPage>{content}</StepPage>
}

const LINK_CLASS = 'font-medium underline underline-offset-4 hover:text-primary'

function CreateAccountForm({
  pending,
  finish,
}: {
  pending: PendingOAuthSignup
  finish: ReturnType<typeof useFinishOAuthSignup>
}) {
  const router = useRouter()
  const { setUser } = useAuthContext()
  // Read as the form shows, so the submit rarely waits on it.
  useLegalVersions()
  const consentVersions = useConsentVersions()
  const ids = useId()
  const [name, setName] = useState(pending.name ?? '')
  const [dateOfBirthParts, setDateOfBirthParts] = useState(EMPTY_DATE_OF_BIRTH)
  const [termsAccepted, setTermsAccepted] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [versionsError, setVersionsError] = useState<'unavailable' | 'changed' | null>(null)
  const [readingVersions, setReadingVersions] = useState(false)
  const termsRef = useRef<HTMLButtonElement>(null)
  const dateOfBirthIdPrefix = `${ids}-dob`

  const dateOfBirth = dateOfBirthFromParts(dateOfBirthParts)
  const termsError = attempted && !termsAccepted
  // Still busy once the account exists: the page is on its way out.
  const busy = readingVersions || finish.isPending || finish.isSuccess
  // The refusals the step shows as their own states replace this form, so
  // whatever error is left belongs in its banner.
  const finishMessage = finish.error?.message ?? null

  const submit = async () => {
    if (busy) {
      return
    }
    setAttempted(true)
    // Focus goes to the first field the form refuses, in the order they show.
    if (dateOfBirth === null) {
      focusControl(firstInvalidDateOfBirthPartId(dateOfBirthIdPrefix, dateOfBirthParts))
      return
    }
    if (!termsAccepted) {
      termsRef.current?.focus()
      return
    }
    setVersionsError(null)
    setReadingVersions(true)
    let versions
    try {
      versions = await consentVersions.read()
    } catch (error) {
      setReadingVersions(false)
      setVersionsError(
        error instanceof ConsentVersionsChangedError ? 'changed' : 'unavailable'
      )
      return
    }
    setReadingVersions(false)
    finish.mutate(
      {
        terms_accepted: true,
        terms_version: versions.termsVersion,
        privacy_version: versions.privacyVersion,
        date_of_birth: dateOfBirth,
        // Sent only when changed: an untouched name keeps the provider's
        // first and last names, and only a name the person typed (or
        // cleared) becomes the account's display name.
        ...(name.trim() !== (pending.name ?? '').trim() && { name }),
      },
      {
        onSuccess: data => {
          if (data.user) {
            setUser(data.user)
          }
          // Replace, not push: the step works once, so Back must not land on it.
          router.replace(sanitizeReturnTo(data.return_to ?? FALLBACK_RETURN_TO))
        },
      }
    )
  }

  const termsErrorId = `${ids}-terms-error`

  return (
    <StepCard>
      <StepHeading eyebrow="Last step" title="Create your Psychic Homily account">
        <p className="text-sm text-muted-foreground">
          Check your details, then agree to the Terms to finish.
        </p>
      </StepHeading>

      {versionsError === 'changed' ? (
        <ConsentVersionsChangedNotice />
      ) : versionsError === 'unavailable' ? (
        <InlineErrorBanner>{CONSENT_VERSIONS_UNAVAILABLE}</InlineErrorBanner>
      ) : (
        finishMessage && <InlineErrorBanner>{finishMessage}</InlineErrorBanner>
      )}

      <form
        noValidate
        className="flex flex-col gap-5"
        onSubmit={event => {
          event.preventDefault()
          void submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <p id={`${ids}-google`} className="text-sm font-medium text-foreground">
            Google account
          </p>
          <div
            aria-labelledby={`${ids}-google`}
            className="flex items-center gap-2.5 rounded-md border bg-muted px-3 py-2"
          >
            <GoogleIcon className="h-4 w-4 shrink-0" />
            <span className="min-w-0 break-all font-mono text-[13px] text-foreground">
              {pending.email}
            </span>
          </div>
          <p className="text-xs text-muted-foreground">
            Not you?{' '}
            <a
              href={googleStartUrl({
                returnTo: sanitizeReturnTo(pending.return_to ?? FALLBACK_RETURN_TO),
                selectAccount: true,
              })}
              className={LINK_CLASS}
            >
              Use a different account
            </a>
          </p>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${ids}-name`}>Name</Label>
          <Input
            id={`${ids}-name`}
            name="name"
            autoComplete="name"
            value={name}
            onChange={event => setName(event.target.value)}
          />
        </div>

        <DateOfBirthField
          idPrefix={dateOfBirthIdPrefix}
          value={dateOfBirthParts}
          onChange={setDateOfBirthParts}
          showError={attempted}
        />

        <div className="flex flex-col gap-1">
          <div className="flex items-start gap-3">
            <Checkbox
              ref={termsRef}
              id={`${ids}-terms`}
              checked={termsAccepted}
              onCheckedChange={checked => setTermsAccepted(checked === true)}
              aria-invalid={termsError}
              aria-describedby={termsError ? termsErrorId : undefined}
              className={cn('mt-0.5', termsError && 'border-destructive')}
            />
            <Label
              htmlFor={`${ids}-terms`}
              className="block cursor-pointer text-sm font-normal leading-5"
            >
              I agree to the{' '}
              <Link href="/terms" target="_blank" className={LINK_CLASS}>
                Terms of Service
              </Link>{' '}
              <span className="text-muted-foreground">
                and I acknowledge the{' '}
                <Link href="/privacy" target="_blank" className={LINK_CLASS}>
                  Privacy Policy
                </Link>
              </span>
            </Label>
          </div>
          {termsError && (
            <p id={termsErrorId} role="alert" className="pl-7 text-sm text-destructive">
              {TERMS_REQUIRED_MESSAGE}
            </p>
          )}
        </div>

        <div className="flex flex-col gap-2.5">
          <Button type="submit" className="w-full" disabled={busy || versionsError === 'changed'}>
            {busy ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              'Create account'
            )}
          </Button>
          <p className="text-xs leading-[18px] text-muted-foreground">
            By clicking Create account you agree to the{' '}
            <Link href="/terms" target="_blank" className={LINK_CLASS}>
              Terms of Service
            </Link>{' '}
            and acknowledge the{' '}
            <Link href="/privacy" target="_blank" className={LINK_CLASS}>
              Privacy Policy
            </Link>
            .
          </p>
        </div>
      </form>
    </StepCard>
  )
}

// The lifetime in this copy is contracts.OAuthPendingSignupTTL in the
// backend; TestOAuthSignupExpiredCopyNamesTheLifetime fails when it changes.
function ExpiredCard({ focusHeading, returnTo }: { focusHeading: boolean; returnTo: string }) {
  return (
    <StepCard>
      <StepHeading
        eyebrow="Sign-in expired"
        title="That sign-in link expired. Start again."
        focusOnMount={focusHeading}
      />
      <p className="text-sm leading-[22px] text-foreground">
        This step stays open for 10 minutes and works once. Nothing was saved, so
        continue with Google to pick up where you left off.
      </p>
      <div className="flex flex-col gap-3">
        <GoogleOAuthButton className="w-full" returnTo={returnTo} />
        <p className="text-center text-xs text-muted-foreground">
          Back to{' '}
          <Link href="/auth" className={LINK_CLASS}>
            sign in
          </Link>
        </p>
      </div>
    </StepCard>
  )
}
