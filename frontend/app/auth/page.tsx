'use client'

import {
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { AlertCircle, Loader2, Mail, Lock, User, Eye, EyeOff, Send, CheckCircle2 } from 'lucide-react'
import { useLogin, useRegister, useSendMagicLink } from '@/features/auth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { PasswordStrengthMeter } from '@/components/ui/password-strength-meter'
import { PasskeyLoginButton } from '@/app/auth/_components/passkey-login'
import { PasskeySignupButton } from '@/app/auth/_components/passkey-signup'
import { GoogleOAuthButton } from '@/app/auth/_components/google-oauth-button'
import {
  SignupIntentFooter,
  SignupIntentPanel,
} from '@/app/auth/_components/signup-intent-panel'
import { CheckInboxInterstitial } from '@/app/auth/_components/check-inbox-interstitial'
import { BackupAuthPrompt } from '@/app/auth/_components/backup-auth-prompt'
import { AccountRecoverableMessage } from '@/app/auth/_components/account-recoverable-message'
import type { SignupHandoff } from '@/app/auth/_components/signup-handoff'
import {
  useSignupRequestDeadline,
  type SignupRequestDeadline,
} from '@/app/auth/_components/signup-request-deadline'
import {
  ACCOUNT_INACTIVE_REASON,
  AUTH_ERROR_PARAM,
  AUTH_REASON_PARAM,
  hasGatedIntent,
  isAccountRecoverableRefusal,
  isReauthReason,
  needsSignupConsent,
} from '@/lib/auth-href'
import { isAccountRecoverable, isAuthError, type AuthError } from '@/lib/errors'
import { getUniqueErrors } from '@/lib/utils/formErrors'
import { MIN_SIGNUP_AGE } from '@/lib/legal'
import {
  ConsentVersionsChangedError,
  isConsentRefusal,
  matchesBuildVersions,
  useConsentVersions,
  useLegalVersions,
  type ConsentVersions,
} from '@/features/auth/hooks/useLegalVersions'
import {
  CONSENT_VERSIONS_UNAVAILABLE,
  ConsentRetryNotice,
  ConsentVersionsChangedNotice,
} from '@/app/auth/_components/consent-versions-notice'
import { useGoogleSignupRetry } from '@/app/auth/_components/google-signup-start'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import {
  safeDecodeQueryParam,
  sanitizeReturnTo,
} from './auth-redirect-utils'
import { useAuthArrivalParams } from '@/app/auth/_components/auth-arrival-params'
import {
  focusFirstInvalidField,
  revealFormAlert,
} from '@/app/auth/_components/form-error-focus'

// The id of the message banner an OAuth refusal puts above the auth card.
const URL_ERROR_ID = 'auth-url-error'

// Shown for ACCOUNT_INACTIVE_REASON: fixed copy keyed by the code, never text
// from the URL, saying no more about the account than "unavailable".
const ACCOUNT_INACTIVE_MESSAGE =
  'Your account is unavailable, so the provider was not connected; please contact support.'

// Password validation constants
const MIN_PASSWORD_LENGTH = 12
const MAX_PASSWORD_LENGTH = 128

// Validation schemas
const loginSchema = z.object({
  email: z.string().email('Please enter a valid email address'),
  password: z.string().min(1, 'Password is required'),
})

const signupSchema = z.object({
  email: z.string().email('Please enter a valid email address'),
  password: z
    .string()
    .min(MIN_PASSWORD_LENGTH, `Password must be at least ${MIN_PASSWORD_LENGTH} characters`)
    .max(MAX_PASSWORD_LENGTH, `Password must be no more than ${MAX_PASSWORD_LENGTH} characters`),
  termsAccepted: z
    .boolean()
    .refine((val) => val === true, {
      message: 'You must agree to the Terms of Service and Privacy Policy',
    }),
  ageConfirmed: z
    .boolean()
    .refine((val) => val === true, {
      message: `You must confirm that you are at least ${MIN_SIGNUP_AGE} years old`,
    }),
})

type LoginFormData = z.infer<typeof loginSchema>
type SignupFormData = z.infer<typeof signupSchema>

// Each form's fields paired with the id of the control that takes their
// focus, in the order the form renders them.
const LOGIN_FIELD_IDS = [
  ['email', 'email'],
  ['password', 'password'],
] as const satisfies readonly (readonly [keyof LoginFormData, string])[]
const SIGNUP_FIELD_IDS = [
  ['email', 'signup-email'],
  ['password', 'signup-password'],
  ['termsAccepted', 'terms'],
  ['ageConfirmed', 'age-confirmation'],
] as const satisfies readonly (readonly [keyof SignupFormData, string])[]

// Clears the sticky top bar when an alert is scrolled to the top of the view.
const FORM_ALERT_CLASS = 'scroll-mt-[var(--topbar-height)]'

function LoginForm({ returnTo }: { returnTo: string }) {
  const router = useRouter()
  const loginMutation = useLogin()
  const sendMagicLink = useSendMagicLink()
  const { setUser } = useAuthContext()
  const [showPassword, setShowPassword] = useState(false)
  const [passkeyError, setPasskeyError] = useState<AuthError | null>(null)
  const [magicLinkSent, setMagicLinkSent] = useState(false)
  const [magicLinkError, setMagicLinkError] = useState<string | null>(null)
  // The alert shows the most recent refusal: a password submit clears the
  // passkey error and a passkey refusal resets the password error, so at most
  // one of the two is set.
  const signInError = loginMutation.error ?? passkeyError

  const reportPasskeyError = (error: AuthError) => {
    loginMutation.reset()
    setPasskeyError(error)
  }

  const form = useForm({
    defaultValues: {
      email: '',
      password: '',
    } as LoginFormData,
    onSubmit: async ({ value }) => {
      setPasskeyError(null)
      loginMutation.mutate(value, {
        onSuccess: data => {
          if (data.user) {
            setUser(data.user)
          }
          router.push(returnTo)
        },
      })
    },
    onSubmitInvalid: ({ formApi }) => {
      focusFirstInvalidField(formApi, LOGIN_FIELD_IDS)
    },
    validators: {
      onSubmit: loginSchema,
    },
  })

  const handleSendMagicLink = async () => {
    const email = form.getFieldValue('email')
    if (!email || !z.string().email().safeParse(email).success) {
      setMagicLinkError('Please enter a valid email address first')
      return
    }

    setMagicLinkError(null)
    setMagicLinkSent(false)

    sendMagicLink.mutate(
      { email },
      {
        onSuccess: data => {
          if (data.success) {
            setMagicLinkSent(true)
          } else if (data.error_code === 'EMAIL_NOT_VERIFIED') {
            setMagicLinkError(data.message)
          } else {
            // Generic success message (even if user doesn't exist - for security)
            setMagicLinkSent(true)
          }
        },
        onError: () => {
          setMagicLinkError('Failed to send magic link. Please try again.')
        },
      }
    )
  }

  return (
    <form
      noValidate
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        form.handleSubmit()
      }}
      className="space-y-4"
    >
      {signInError && (
        <Alert ref={revealFormAlert} variant="destructive" className={FORM_ALERT_CLASS}>
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            {isAccountRecoverable(signInError) ? (
              <AccountRecoverableMessage />
            ) : (
              signInError.message
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* OAuth and Passkey login options */}
      <div className="space-y-3">
        <GoogleOAuthButton
          className="w-full"
          variant="login"
          returnTo={returnTo}
        />
        <PasskeyLoginButton
          onError={reportPasskeyError}
          className="w-full"
          returnTo={returnTo}
        />
        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <span className="w-full border-t" />
          </div>
          <div className="relative flex justify-center text-xs uppercase">
            <span className="bg-card px-2 text-muted-foreground">Or continue with email</span>
          </div>
        </div>
      </div>

      <form.Field name="email">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={field.name}>Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={field.name}
                name={field.name}
                type="email"
                placeholder="you@example.com"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => {
                  field.handleChange(e.target.value)
                  // Reset magic link states when email changes
                  setMagicLinkSent(false)
                  setMagicLinkError(null)
                }}
                className="pl-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
            </div>
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="password">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={field.name}>Password</Label>
            <div className="relative">
              <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={field.name}
                name={field.name}
                type={showPassword ? 'text' : 'password'}
                placeholder="Enter your password"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => field.handleChange(e.target.value)}
                className="pl-10 pr-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showPassword}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
            <div className="flex justify-end">
              <button
                type="button"
                onClick={handleSendMagicLink}
                className="text-xs text-muted-foreground hover:text-primary underline-offset-4 hover:underline"
                disabled={sendMagicLink.isPending}
              >
                Forgot password?
              </button>
            </div>
          </div>
        )}
      </form.Field>

      <form.Subscribe selector={state => state.isSubmitting}>
        {isSubmitting => (
          <Button
            type="submit"
            className="w-full"
            disabled={isSubmitting || loginMutation.isPending}
          >
            {isSubmitting || loginMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Signing in...
              </>
            ) : (
              'Sign in'
            )}
          </Button>
        )}
      </form.Subscribe>

      {/* Magic link option */}
      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t" />
        </div>
        <div className="relative flex justify-center text-xs">
          <span className="bg-card px-2 text-muted-foreground">or</span>
        </div>
      </div>

      {magicLinkSent ? (
        <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
          <CheckCircle2 className="h-4 w-4 shrink-0" />
          <span>Check your email for a sign-in link. It expires in 15 minutes.</span>
        </div>
      ) : (
        <>
          {magicLinkError && (
            <div role="alert" className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{magicLinkError}</span>
            </div>
          )}
          <Button
            type="button"
            variant="outline"
            className="w-full"
            onClick={handleSendMagicLink}
            disabled={sendMagicLink.isPending}
          >
            {sendMagicLink.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Sending...
              </>
            ) : (
              <>
                <Send className="h-4 w-4" />
                Email me a sign-in link
              </>
            )}
          </Button>
        </>
      )}
    </form>
  )
}

/** A password sign-up request in flight, as the form that sent it tracks it. */
interface PasswordSignupRequest {
  /** The claim it made on the page. */
  claim: SignupHandoff
  /** The signal that stands for it under the page's deadline. */
  deadlineSignal: AbortSignal
  /** What was submitted, for the one retry a consent refusal gets. */
  value: SignupFormData
  /** This request is that retry. */
  consentRetried: boolean
}

interface SignupFormProps {
  returnTo: string
  /**
   * Reports the signup tab's claim on the page, for both the password form and
   * the passkey button. The page swaps in what follows the sign-up rather than
   * navigating, so the address never has to travel through the URL. `null`
   * releases the claim. Takes an updater too, so a request can settle a claim
   * against the one the page holds when it lands.
   */
  onHandoffChange: Dispatch<SetStateAction<SignupHandoff | null>>
  /**
   * Bounds the register request and, apart from it, the profile refetch that
   * follows an accepted registration.
   */
  passwordRequestDeadline: SignupRequestDeadline
  /** Bounds the passkey sign-up button's requests. */
  passkeyRequestDeadline: SignupRequestDeadline
  /**
   * The form is showing as the arrival's own tab, and the arrival is an OAuth
   * sign-in refused for want of the consent this form records. The form then
   * centres the terms checkbox in the viewport and focuses it, each time the
   * page presents that arrival, a re-show after the page was hidden included.
   * Centring is what puts "Sign up with Google" above the checkbox and the age
   * confirmation below it on a phone screen, and only while no taller content
   * sits between them.
   */
  consentRequested: boolean
  /**
   * The id of the element stating why consent was asked for, which the terms
   * checkbox names as its description while consentRequested holds.
   */
  consentReasonId?: string
}

function SignupForm({
  returnTo,
  onHandoffChange,
  passwordRequestDeadline,
  passkeyRequestDeadline,
  consentRequested,
  consentReasonId,
}: SignupFormProps) {
  const { setUser } = useAuthContext()
  // The password request in flight: the claim it made and the signal that
  // stands for it under the page's deadline. The submit button is disabled
  // while it is in flight, so there is one per form instance. It outlives the
  // form, and it touches neither a claim nor a deadline another request holds:
  // the passkey path's, or a remounted form's.
  const passwordRequestRef = useRef<PasswordSignupRequest | null>(null)
  // Settles the claim this request made, or no claim once its deadline has
  // released that one.
  const settlePasswordClaim = (next: SignupHandoff | null) => {
    const claim = passwordRequestRef.current?.claim
    onHandoffChange(current =>
      current === claim || current === null ? next : current
    )
  }
  // On the hook, not on `mutate`: the page stops rendering this form once the
  // profile reports the new session while the claim is pending.
  const registerMutation = useRegister({
    onSuccess: (data, variables) => {
      if (!data.user) {
        // No session to hand off to. Release the claim so the page behaves
        // exactly as it did before the attempt.
        settlePasswordClaim(null)
        return
      }

      setUser(data.user)
      settlePasswordClaim({
        status: 'complete',
        email: data.user.email || variables.email,
        method: 'password',
      })
    },
    onError: error => {
      settlePasswordClaim(null)
      // Refused with versions the backend published: the read was older than
      // the backend's answer. Retried once, and only from a form still shown;
      // a second refusal is shown as it is.
      const request = passwordRequestRef.current
      if (
        mountedRef.current &&
        request &&
        !request.consentRetried &&
        isAuthError(error) &&
        isConsentRefusal(error.code)
      ) {
        void retryAfterConsentRefusal(request.value)
      }
    },
    onSettled: () => {
      const request = passwordRequestRef.current
      if (request) {
        passwordRequestDeadline.disarm(request.deadlineSignal)
      }
    },
  })
  // Read as the form shows, so a submit rarely waits on it, and so a backend
  // publishing versions other than the ones this page's legal links show
  // blocks every sign-up control before anyone fills the form in.
  const { data: publishedVersions } = useLegalVersions()
  const versionsChanged =
    publishedVersions !== undefined && !matchesBuildVersions(publishedVersions)
  const consentVersions = useConsentVersions()
  const [showPassword, setShowPassword] = useState(false)
  const [passwordValue, setPasswordValue] = useState('')
  const [passkeyError, setPasskeyError] = useState<string | null>(null)
  const [oauthError, setOauthError] = useState<string | null>(null)
  const [consentVersionsError, setConsentVersionsError] = useState<string | null>(null)
  const [isRetryingConsent, setIsRetryingConsent] = useState(false)
  // The form's banner shows one message: the latest one set through here.
  const showError = (
    set: (message: string | null) => void,
    message: string | null
  ) => {
    setConsentVersionsError(null)
    setPasskeyError(null)
    setOauthError(null)
    set(message)
  }
  const [hasAttemptedSubmit, setHasAttemptedSubmit] = useState(false)
  const termsCheckboxRef = useRef<HTMLButtonElement>(null)
  // A retry still waiting on its versions when the form goes away is dropped,
  // so no registration goes out from a form nobody can see.
  const mountedRef = useRef(false)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    const termsCheckbox = termsCheckboxRef.current
    if (!consentRequested || !termsCheckbox) {
      return
    }
    termsCheckbox.scrollIntoView({ block: 'center' })
    termsCheckbox.focus({ preventScroll: true })
  }, [consentRequested])

  const register = (
    value: SignupFormData,
    versions: ConsentVersions,
    consentRetried: boolean
  ) => {
    // Claimed before the request goes out, not after it lands. See the
    // SignupHandoff doc comment for the redirect race this closes.
    const request: PasswordSignupRequest = {
      claim: { status: 'pending', email: value.email },
      deadlineSignal: passwordRequestDeadline.arm(),
      value,
      consentRetried,
    }
    passwordRequestRef.current = request
    onHandoffChange(request.claim)

    registerMutation.mutate({
      email: value.email,
      password: value.password,
      terms_accepted: value.termsAccepted,
      terms_version: versions.termsVersion,
      privacy_version: versions.privacyVersion,
      age_confirmed: value.ageConfirmed,
      min_age_attested: MIN_SIGNUP_AGE,
      signal: request.deadlineSignal,
      // The profile refetch that follows takes no signal: this deadline
      // bounds only how long the claim waits on it.
      onRegistered: () => {
        request.deadlineSignal = passwordRequestDeadline.rearm(request.deadlineSignal)
      },
    })
  }

  // The notice covers only the re-read: it ends before the retried request
  // goes out, whose own pending state and answer the form shows as usual, so
  // nothing that skips a mutation callback can leave it up.
  const retryAfterConsentRefusal = async (value: SignupFormData) => {
    setIsRetryingConsent(true)
    let versions: ConsentVersions
    try {
      versions = await consentVersions.refresh()
    } catch (error) {
      setIsRetryingConsent(false)
      // Versions other than this build's are never sent; the form shows
      // why from the read that found them.
      if (!(error instanceof ConsentVersionsChangedError)) {
        showError(setConsentVersionsError, CONSENT_VERSIONS_UNAVAILABLE)
      }
      return
    }
    if (mountedRef.current) {
      register(value, versions, true)
    }
    setIsRetryingConsent(false)
  }

  const form = useForm({
    defaultValues: {
      email: '',
      password: '',
      termsAccepted: false,
      ageConfirmed: false,
    } as SignupFormData,
    onSubmit: async ({ value }) => {
      setConsentVersionsError(null)
      let versions: ConsentVersions
      try {
        versions = await consentVersions.read()
      } catch (error) {
        // Sent only once the backend confirms this build's versions; a read
        // that found others leaves the form showing why.
        if (!(error instanceof ConsentVersionsChangedError)) {
          showError(setConsentVersionsError, CONSENT_VERSIONS_UNAVAILABLE)
        }
        return
      }
      register(value, versions, false)
    },
    onSubmitInvalid: ({ formApi }) => {
      focusFirstInvalidField(formApi, SIGNUP_FIELD_IDS)
    },
    validators: {
      ...(hasAttemptedSubmit && { onChange: signupSchema }),
      onSubmit: signupSchema,
    },
  })

  return (
    <form
      noValidate
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        setHasAttemptedSubmit(true)
        form.handleSubmit()
      }}
      className="space-y-4"
    >
      {versionsChanged ? (
        <ConsentVersionsChangedNotice />
      ) : isRetryingConsent ? (
        <ConsentRetryNotice />
      ) : (
        (consentVersionsError || registerMutation.error || passkeyError || oauthError) && (
          <Alert ref={revealFormAlert} variant="destructive" className={FORM_ALERT_CLASS}>
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              {consentVersionsError || registerMutation.error?.message || passkeyError || oauthError}
            </AlertDescription>
          </Alert>
        )
      )}

      {/* OAuth and Passkey signup options */}
      <div className="space-y-3">
        <form.Subscribe selector={state => [state.values.termsAccepted, state.values.ageConfirmed]}>
          {([termsAccepted, ageConfirmed]) => (
            <GoogleOAuthButton
              className="w-full"
              variant="signup"
              termsAccepted={termsAccepted}
              ageConfirmed={ageConfirmed}
              disabled={versionsChanged}
              returnTo={returnTo}
              onMissingTermsAcceptance={() => {
                showError(setOauthError, 'You must agree to the Terms of Service and Privacy Policy before continuing with Google')
              }}
              onMissingAgeConfirmation={() => {
                showError(setOauthError, `You must confirm that you are at least ${MIN_SIGNUP_AGE} years old before continuing with Google`)
              }}
              onConsentVersionsUnavailable={() => {
                showError(setOauthError, CONSENT_VERSIONS_UNAVAILABLE)
              }}
            />
          )}
        </form.Subscribe>
        <PasskeySignupButton
          onError={message => showError(setPasskeyError, message)}
          onHandoffChange={onHandoffChange}
          requestDeadline={passkeyRequestDeadline}
          className="w-full"
          minAge={MIN_SIGNUP_AGE}
          versionsChanged={versionsChanged}
        />
        <div className="relative">
          <div className="absolute inset-0 flex items-center">
            <span className="w-full border-t" />
          </div>
          <div className="relative flex justify-center text-xs uppercase">
            <span className="bg-card px-2 text-muted-foreground">Or continue with email</span>
          </div>
        </div>
      </div>

      <form.Field name="email">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={`signup-${field.name}`}>Email</Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={`signup-${field.name}`}
                name={field.name}
                type="email"
                placeholder="you@example.com"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => field.handleChange(e.target.value)}
                className="pl-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
            </div>
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="password">
        {field => (
          <div className="space-y-2">
            <Label htmlFor={`signup-${field.name}`}>Password</Label>
            <div className="relative">
              <Lock className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={`signup-${field.name}`}
                name={field.name}
                type={showPassword ? 'text' : 'password'}
                placeholder="Create a password"
                value={field.state.value}
                onBlur={field.handleBlur}
                onChange={e => {
                  field.handleChange(e.target.value)
                  setPasswordValue(e.target.value)
                }}
                className="pl-10 pr-10"
                aria-invalid={field.state.meta.errors.length > 0}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showPassword}
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
            {/* Password strength meter with requirements */}
            <PasswordStrengthMeter password={passwordValue} showRequirements={true} />
          </div>
        )}
      </form.Field>

      <form.Field name="termsAccepted">
        {field => (
          <div className="space-y-2">
            <div className="flex items-start space-x-3">
              <Checkbox
                ref={termsCheckboxRef}
                id="terms"
                aria-describedby={consentRequested ? consentReasonId : undefined}
                checked={field.state.value}
                onCheckedChange={(checked) => {
                  const accepted = checked === true
                  field.handleChange(accepted)
                  if (accepted) {
                    setOauthError(null)
                  }
                }}
                aria-invalid={field.state.meta.errors.length > 0}
                className="mt-0.5"
              />
              <Label
                htmlFor="terms"
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
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Field name="ageConfirmed">
        {field => (
          <div className="space-y-2">
            <div className="flex items-start space-x-3">
              <Checkbox
                id="age-confirmation"
                checked={field.state.value}
                onCheckedChange={(checked) => {
                  const confirmed = checked === true
                  field.handleChange(confirmed)
                  if (confirmed) {
                    setOauthError(null)
                  }
                }}
                aria-invalid={field.state.meta.errors.length > 0}
                className="mt-0.5"
              />
              <Label
                htmlFor="age-confirmation"
                className="text-sm font-normal leading-relaxed cursor-pointer"
              >
                I confirm that I am at least {MIN_SIGNUP_AGE} years old
              </Label>
            </div>
            {field.state.meta.errors.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {getUniqueErrors(field.state.meta.errors)}
              </p>
            )}
          </div>
        )}
      </form.Field>

      <form.Subscribe selector={state => state.isSubmitting}>
        {isSubmitting => (
          <Button
            type="submit"
            className="w-full"
            disabled={isSubmitting || registerMutation.isPending || isRetryingConsent || versionsChanged}
          >
            {isSubmitting || registerMutation.isPending || isRetryingConsent ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              'Create account'
            )}
          </Button>
        )}
      </form.Subscribe>
    </form>
  )
}

function AuthPageContent() {
  const router = useRouter()
  // The query string as it arrived. The banner parameters leave the address
  // bar after the first render and stay here, so a reload does not replay one.
  const liveSearchParams = useSearchParams()
  const searchParams = useAuthArrivalParams(liveSearchParams)
  const { authStatus, isLoading } = useAuthContext()

  // Get error from URL query params (e.g., OAuth errors)
  const urlError = safeDecodeQueryParam(searchParams.get(AUTH_ERROR_PARAM))
  // Shown in the page's own words with its recovery link, whatever ?error says.
  const urlRefusalRecoverable = isAccountRecoverableRefusal(searchParams)

  // Get returnTo from URL query params (for redirecting after login)
  const returnTo = sanitizeReturnTo(searchParams.get('returnTo'))

  // A re-authentication, not a sign-in. The reader arrives here already
  // authenticated when a security-relevant change needs the account proved
  // again: the OAuth link path redirects them, and a refused credential mint
  // offers them the link. Either way the redirect below has to be held back,
  // because bouncing them straight to returnTo would return them to the control
  // that just refused them, with nothing changed and no way to tell why.
  const reason = searchParams.get(AUTH_REASON_PARAM)
  const isReauth = isReauthReason(reason)
  const isAccountInactive = reason === ACCOUNT_INACTIVE_REASON

  // Two arrivals open on Create account: a gated control naming the action it
  // refused an anonymous viewer, and an OAuth sign-in refused for want of the
  // consent only that tab records, whose message the banner above the card
  // shows. Every other route in opens on Sign in, re-auth included.
  //
  // The default belongs to the arrival, not to the mount. Neither a
  // navigation that only changes the query string nor leaving and coming back
  // remounts this page, so a tab the viewer picks lasts only while both hold:
  // the query string it was picked under is still the one in the URL (any
  // change to it, including one this page makes, drops the pick), and the
  // page has not been hidden since (the effect below drops it on hide). Tab
  // changes are never written back to the URL.
  const arrival = searchParams.toString()
  const isConsentArrival = needsSignupConsent(searchParams) && !isReauth
  const arrivalTab =
    (hasGatedIntent(searchParams) && !isReauth) || isConsentArrival
      ? 'signup'
      : 'login'
  const [pickedTab, setPickedTab] = useState<{
    arrival: string
    tab: string
  } | null>(null)
  if (pickedTab !== null && pickedTab.arrival !== arrival) {
    setPickedTab(null)
  }
  const showingArrivalTab = pickedTab?.arrival !== arrival
  const activeTab = showingArrivalTab ? arrivalTab : pickedTab.tab
  const setActiveTab = (tab: string) => setPickedTab({ arrival, tab })
  const [signupHandoff, setSignupHandoff] = useState<SignupHandoff | null>(null)
  // A re-authentication link can outlive the session it was sent for, so a
  // viewer with no session gets Create account too. A sign-up completed under
  // that arrival ends its re-authentication: the new account has no earlier
  // session to prove, so the page treats the viewer as any other new account
  // and the redirect below sends them on to returnTo.
  const [signedUpArrival, setSignedUpArrival] = useState<string | null>(null)
  if (signupHandoff?.status === 'complete' && signedUpArrival !== arrival) {
    setSignedUpArrival(arrival)
  }
  const holdsReauth = isReauth && signedUpArrival !== arrival
  // Creating an account is not a way to prove you hold one, so an
  // authenticated viewer proving theirs is offered Sign in alone, unless a
  // sign-up begun on this page still holds its claim.
  const offersSignup =
    !isReauth || authStatus !== 'authenticated' || signupHandoff !== null
  // A request that never answers leaves its claim pending, and an
  // authenticated viewer with a pending claim gets neither the form nor the
  // redirect, so a request past its deadline releases the claim. The two
  // sign-up paths can each have a request in flight at once, so each has its
  // own deadline.
  const releasePendingSignup = useCallback(() => {
    setSignupHandoff(current => (current?.status === 'pending' ? null : current))
  }, [])
  const passwordSignupDeadline = useSignupRequestDeadline(releasePendingSignup)
  const passkeySignupDeadline = useSignupRequestDeadline(releasePendingSignup)
  const signInTabRef = useRef<HTMLButtonElement>(null)
  // A consent arrival answering a Google sign-up start from this tab is
  // retried once instead of asking for consent the reader already gave.
  // The hydration render cannot see whether a retry is due, so the consent
  // controls are focused only from the render after it.
  const hydrated = useHydrated()
  const googleSignupRetry = useGoogleSignupRetry({
    arrival,
    consentArrival: isConsentArrival && authStatus !== 'authenticated',
    returnTo,
  })

  // A hidden page runs its effect cleanups, and a shown one reruns the
  // effects, so this cleanup is the page's "hidden" signal.
  useEffect(() => () => setPickedTab(null), [])

  // Redirect if already authenticated.
  //
  // A registering user becomes authenticated partway through the signup
  // request, so the signup tab's claim has to hold this back. Otherwise the
  // redirect fires the moment the session exists and the user never learns an
  // email is waiting. The claim is in-memory only: on a reload it is gone and
  // the redirect resumes, so it cannot strand anyone.
  useEffect(() => {
    if (authStatus === 'authenticated' && !signupHandoff && !holdsReauth) {
      router.push(returnTo)
    }
  }, [authStatus, router, returnTo, signupHandoff, holdsReauth])

  // `isLoading`, NOT `authStatus === 'pending'`, and this is the one page
  // where that is the right way round.
  //
  // 'pending' is terminal for a profile fetch that failed without answering
  // (429, 5xx, network, 403): `useProfile` retries twice, gives up, and only a
  // throttled focus or reconnect refetch moves it. Withholding this form there
  // strands the viewer who most needs it, since reaching 'pending' takes a
  // cookie the backend did not answer for and that cookie may name a dead
  // session. `isLoading` goes false as soon as the query errors, so the form
  // comes back. It is the same rule the AuthStatus docblock states for the nav
  // link: a sign-in affordance claims nothing about who is looking, and the
  // unsettled window must not take it away.
  //
  // The cost is the mirror case: a signed-in viewer whose profile has not
  // landed sees a sign-in form. They lose nothing by it, and the effect above
  // redirects them the moment the status settles.
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Every way of closing the prompt moves the claim on to its check-inbox
  // step, which the block below renders. The claim stays held, and so does
  // the redirect.
  if (
    signupHandoff?.status === 'complete' &&
    signupHandoff.method === 'passkey' &&
    signupHandoff.step === 'backup-prompt'
  ) {
    return (
      <div className="min-h-[calc(100vh-64px)]">
        <BackupAuthPrompt
          open
          onOpenChange={open => {
            if (!open) setSignupHandoff({ ...signupHandoff, step: 'check-inbox' })
          }}
        />
      </div>
    )
  }

  if (signupHandoff?.status === 'complete') {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
        <div className="w-full max-w-md">
          <CheckInboxInterstitial
            email={signupHandoff.email}
            returnTo={returnTo}
          />
        </div>
      </div>
    )
  }

  // Don't render the form if authenticated (will redirect).
  //
  // Except on a re-authentication, where the whole point is to show the form
  // to someone who IS signed in. Completing any factor mints a fresh session,
  // which is what the caller that sent them here was waiting for.
  if (authStatus === 'authenticated' && !holdsReauth) {
    return null
  }

  // What the banner above the card says, most specific first. Versions that
  // changed are explained by the sign-up form's own notice, not here.
  const urlBanner =
    googleSignupRetry === 'changed' ? null : googleSignupRetry === 'unavailable' ? (
      CONSENT_VERSIONS_UNAVAILABLE
    ) : urlRefusalRecoverable ? (
      <AccountRecoverableMessage />
    ) : (
      urlError
    )

  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <User className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">
            {isReauth ? "Confirm it's you" : 'Welcome to Psychic Homily'}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {isReauth
              ? 'Sign in again to connect a provider to your account.'
              : "You're welcome to join our community."}
          </p>
        </div>

        {/* OAuth/URL Error Display */}
        {googleSignupRetry === 'retrying' ? (
          <ConsentRetryNotice className="mb-6" />
        ) : (
          urlBanner && (
            <Alert variant="destructive" className="mb-6">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription id={URL_ERROR_ID}>{urlBanner}</AlertDescription>
            </Alert>
          )
        )}

        {isAccountInactive && (
          <Alert variant="destructive" className="mb-6">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{ACCOUNT_INACTIVE_MESSAGE}</AlertDescription>
          </Alert>
        )}

        {/* Auth Card with Tabs */}
        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="pb-4">
            <Tabs
              value={offersSignup ? activeTab : 'login'}
              onValueChange={setActiveTab}
              className="w-full"
            >
              <TabsList
                className={
                  offersSignup ? 'grid w-full grid-cols-2' : 'grid w-full grid-cols-1'
                }
              >
                <TabsTrigger ref={signInTabRef} value="login">Sign in</TabsTrigger>
                {offersSignup && (
                  <TabsTrigger value="signup">Create account</TabsTrigger>
                )}
              </TabsList>

              <TabsContent value="login" className="mt-6">
                <CardTitle className="text-lg">
                  {isReauth
                    ? 'Confirm it\'s you to connect a provider'
                    : 'Sign in to your account'}
                </CardTitle>
                <CardDescription className="mt-1">
                  {isReauth
                    ? 'Use whichever method this account already has.'
                    : 'Enter your email and password to continue'}
                </CardDescription>
                <div className="mt-4">
                  <LoginForm returnTo={returnTo} />
                </div>
              </TabsContent>

              <TabsContent value="signup" className="mt-6">
                <SignupIntentPanel />
                <div className="mt-6">
                  <SignupForm
                    returnTo={returnTo}
                    onHandoffChange={setSignupHandoff}
                    passwordRequestDeadline={passwordSignupDeadline}
                    passkeyRequestDeadline={passkeySignupDeadline}
                    consentRequested={
                      hydrated &&
                      showingArrivalTab &&
                      isConsentArrival &&
                      googleSignupRetry !== 'retrying' &&
                      googleSignupRetry !== 'changed'
                    }
                    consentReasonId={
                      urlError || googleSignupRetry === 'unavailable'
                        ? URL_ERROR_ID
                        : undefined
                    }
                  />
                </div>
                <div className="mt-4">
                  <SignupIntentFooter
                    onSignInClick={() => {
                      // Radix unmounts this panel on the tab change, taking the
                      // focused button with it. The tab list itself never
                      // unmounts, so hand focus over first and land the user
                      // where clicking the tab directly would have.
                      signInTabRef.current?.focus()
                      setActiveTab('login')
                    }}
                  />
                </div>
              </TabsContent>
            </Tabs>
          </CardHeader>
        </Card>

        {/* Footer */}
        <p className="mt-6 text-center text-xs text-muted-foreground">
          By signing in, you agree to our{' '}
          <Link href="/terms" className="underline underline-offset-4 hover:text-primary">
            Terms of Service
          </Link>{' '}
          and{' '}
          <Link href="/privacy" className="underline underline-offset-4 hover:text-primary">
            Privacy Policy
          </Link>
          .
        </p>

        {/* Recovery link */}
        <p className="mt-4 text-center text-xs text-muted-foreground">
          Deleted your account?{' '}
          <Link href="/auth/recover" className="underline underline-offset-4 hover:text-primary">
            Recover it here
          </Link>
        </p>
      </div>
    </div>
  )
}

export default function AuthPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    }>
      <AuthPageContent />
    </Suspense>
  )
}
