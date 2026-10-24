'use client'

import { useId, useState, type ReactNode } from 'react'
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
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'

export const TERMS_REQUIRED_MESSAGE =
  'Agree to the Terms of Service to create your account.'

/** The refusal the backend gives a date that is not a plausible date of birth. */
export const DATE_OF_BIRTH_INVALID_MESSAGE = 'Enter a valid date of birth.'

const LOAD_FAILED_MESSAGE =
  'Service temporarily unavailable. Please try again.'

/**
 * The month, day and year as typed, as the YYYY-MM-DD date the backend reads,
 * or null when they do not name a real calendar date. The backend judges the
 * age and the plausible range; this only keeps an impossible date from
 * leaving the page.
 */
export function dateOfBirthFromParts(
  month: string,
  day: string,
  year: string
): string | null {
  if (!/^\d{1,2}$/.test(month) || !/^\d{1,2}$/.test(day) || !/^\d{4}$/.test(year)) {
    return null
  }
  const m = Number(month)
  const d = Number(day)
  const y = Number(year)
  const date = new Date(Date.UTC(y, m - 1, d))
  if (
    date.getUTCFullYear() !== y ||
    date.getUTCMonth() !== m - 1 ||
    date.getUTCDate() !== d
  ) {
    return null
  }
  return `${year}-${month.padStart(2, '0')}-${day.padStart(2, '0')}`
}

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
    content = <AgeRefusedCard />
  } else if (
    pending.data.status === 'expired' ||
    finishCode === AuthErrorCode.OAUTH_SIGNUP_EXPIRED
  ) {
    content = <ExpiredCard />
  } else {
    content = <CreateAccountForm pending={pending.data} finish={finish} />
  }

  return (
    <div className="flex min-h-[calc(100vh-64px)] justify-center px-4 pb-16 pt-12">
      <div className="w-full max-w-md">{content}</div>
    </div>
  )
}

function StepCard({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-col gap-5 rounded-lg border bg-card p-5 text-card-foreground sm:p-8">
      {children}
    </div>
  )
}

function StepHeading({
  eyebrow,
  title,
  children,
}: {
  eyebrow: string
  title: string
  children?: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2">
      <p className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
        {eyebrow}
      </p>
      <h1 className="text-2xl font-bold leading-[30px] text-foreground">{title}</h1>
      {children}
    </div>
  )
}

const LEGAL_LINK_CLASS = 'font-medium underline underline-offset-4 hover:text-primary'

function CreateAccountForm({
  pending,
  finish,
}: {
  pending: PendingOAuthSignup
  finish: ReturnType<typeof useFinishOAuthSignup>
}) {
  const router = useRouter()
  const { setUser } = useAuthContext()
  const consentVersions = useConsentVersions()
  const ids = useId()
  const [name, setName] = useState(pending.name ?? '')
  const [month, setMonth] = useState('')
  const [day, setDay] = useState('')
  const [year, setYear] = useState('')
  const [termsAccepted, setTermsAccepted] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [versionsError, setVersionsError] = useState<'unavailable' | 'changed' | null>(null)
  const [readingVersions, setReadingVersions] = useState(false)

  const dateOfBirth = dateOfBirthFromParts(month, day, year)
  const termsError = attempted && !termsAccepted
  const dateError = attempted && dateOfBirth === null
  const busy = readingVersions || finish.isPending
  // The refusals the step shows as their own states never reach the banner.
  const finishMessage =
    finish.error &&
    !(
      isAuthError(finish.error) &&
      (finish.error.code === AuthErrorCode.AGE_CONFIRMATION_REQUIRED ||
        finish.error.code === AuthErrorCode.OAUTH_SIGNUP_EXPIRED)
    )
      ? finish.error.message
      : null

  const submit = async () => {
    setAttempted(true)
    if (!termsAccepted || dateOfBirth === null) {
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
        name,
      },
      {
        onSuccess: data => {
          if (data.user) {
            setUser(data.user)
          }
          router.push(sanitizeReturnTo(data.return_to ?? FALLBACK_RETURN_TO))
        },
      }
    )
  }

  const hintId = `${ids}-dob-hint`
  const dateErrorId = `${ids}-dob-error`
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
              href={googleStartUrl({ returnTo: FALLBACK_RETURN_TO, selectAccount: true })}
              className="font-medium underline underline-offset-4 hover:text-primary"
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

        <fieldset className="flex flex-col gap-2" aria-describedby={dateError ? `${hintId} ${dateErrorId}` : hintId}>
          <legend className="mb-2 text-sm font-medium text-foreground">Date of birth</legend>
          <p id={hintId} className="text-xs text-muted-foreground">
            Never shown on your profile.
          </p>
          <div className="flex items-start gap-3">
            <DatePart
              id={`${ids}-dob-month`}
              label="Month"
              placeholder="MM"
              autoComplete="bday-month"
              maxLength={2}
              className="w-16"
              value={month}
              onChange={setMonth}
              invalid={dateError}
            />
            <DatePart
              id={`${ids}-dob-day`}
              label="Day"
              placeholder="DD"
              autoComplete="bday-day"
              maxLength={2}
              className="w-16"
              value={day}
              onChange={setDay}
              invalid={dateError}
            />
            <DatePart
              id={`${ids}-dob-year`}
              label="Year"
              placeholder="YYYY"
              autoComplete="bday-year"
              maxLength={4}
              className="w-[88px]"
              value={year}
              onChange={setYear}
              invalid={dateError}
            />
          </div>
          {dateError && (
            <p id={dateErrorId} role="alert" className="text-sm text-destructive">
              {DATE_OF_BIRTH_INVALID_MESSAGE}
            </p>
          )}
        </fieldset>

        <div className="flex flex-col gap-1">
          <div className="flex items-start gap-3">
            <Checkbox
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
              <Link href="/terms" target="_blank" className={LEGAL_LINK_CLASS}>
                Terms of Service
              </Link>{' '}
              <span className="text-muted-foreground">
                and I acknowledge the{' '}
                <Link href="/privacy" target="_blank" className={LEGAL_LINK_CLASS}>
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
            <Link href="/terms" target="_blank" className={LEGAL_LINK_CLASS}>
              Terms of Service
            </Link>{' '}
            and acknowledge the{' '}
            <Link href="/privacy" target="_blank" className={LEGAL_LINK_CLASS}>
              Privacy Policy
            </Link>
            .
          </p>
        </div>
      </form>
    </StepCard>
  )
}

function DatePart({
  id,
  label,
  placeholder,
  autoComplete,
  maxLength,
  className,
  value,
  onChange,
  invalid,
}: {
  id: string
  label: string
  placeholder: string
  autoComplete: string
  maxLength: number
  className: string
  value: string
  onChange: (value: string) => void
  invalid: boolean
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-xs text-muted-foreground">
        {label}
      </label>
      <Input
        id={id}
        inputMode="numeric"
        autoComplete={autoComplete}
        placeholder={placeholder}
        maxLength={maxLength}
        className={className}
        value={value}
        aria-invalid={invalid}
        onChange={event => onChange(event.target.value.replace(/\D/g, ''))}
      />
    </div>
  )
}

function ExpiredCard() {
  return (
    <StepCard>
      <StepHeading eyebrow="Sign-in expired" title="That sign-in link expired. Start again." />
      <p className="text-sm leading-[22px] text-foreground">
        This step stays open for 10 minutes and works once. Nothing was saved, so
        continue with Google to pick up where you left off.
      </p>
      <div className="flex flex-col gap-3">
        <GoogleOAuthButton className="w-full" />
        <p className="text-center text-xs text-muted-foreground">
          Back to{' '}
          <Link href="/auth" className="font-medium underline underline-offset-4 hover:text-primary">
            sign in
          </Link>
        </p>
      </div>
    </StepCard>
  )
}

function AgeRefusedCard() {
  return (
    <StepCard>
      <StepHeading eyebrow="Account not created" title="We can't create an account for you." />
      <p className="text-sm leading-[22px] text-foreground">
        Psychic Homily accounts are for people 16 and over, so we haven&apos;t
        created one. You can still browse shows, artists and venues without an
        account.
      </p>
      <Button asChild className="w-full">
        <Link href="/shows">Browse upcoming shows</Link>
      </Button>
    </StepCard>
  )
}
