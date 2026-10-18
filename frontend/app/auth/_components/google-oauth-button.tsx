'use client'

import { useEffect, useRef, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { FALLBACK_RETURN_TO } from '@/lib/auth-href'
import { useConsentVersions } from '@/features/auth/hooks/useLegalVersions'
import {
  forgetGoogleSignupStart,
  googleStartUrl,
  rememberGoogleSignupStart,
} from './google-signup-start'

interface GoogleOAuthButtonProps {
  className?: string
  variant?: 'login' | 'signup'
  termsAccepted?: boolean
  onMissingTermsAcceptance?: () => void
  // PSY-1023: required minimum-age confirmation, mirroring terms acceptance.
  ageConfirmed?: boolean
  onMissingAgeConfirmation?: () => void
  /**
   * A sign-up start was not made because the published consent versions could
   * not be read.
   */
  onConsentVersionsUnavailable?: () => void
  /**
   * Where the reader lands after signing in, already through
   * sanitizeReturnTo. See googleStartUrl for what the backend does with it.
   */
  returnTo?: string
}

export function GoogleOAuthButton({
  className,
  variant = 'login',
  termsAccepted = false,
  onMissingTermsAcceptance,
  ageConfirmed = false,
  onMissingAgeConfirmation,
  onConsentVersionsUnavailable,
  returnTo = FALLBACK_RETURN_TO,
}: GoogleOAuthButtonProps) {
  const consentVersions = useConsentVersions()
  const [isReadingVersions, setIsReadingVersions] = useState(false)
  // A start whose versions arrive after the button is gone is dropped, so it
  // cannot replace whatever the reader did next.
  const mountedRef = useRef(false)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  const handleGoogleLogin = async () => {
    if (variant === 'login') {
      // A sign-in is never retried as a sign-up, whatever it is refused with.
      forgetGoogleSignupStart()
      window.location.href = googleStartUrl({ returnTo })
      return
    }
    if (!termsAccepted) {
      onMissingTermsAcceptance?.()
      return
    }
    if (!ageConfirmed) {
      onMissingAgeConfirmation?.()
      return
    }

    setIsReadingVersions(true)
    let versions
    try {
      versions = await consentVersions.read()
    } catch {
      setIsReadingVersions(false)
      if (mountedRef.current) onConsentVersionsUnavailable?.()
      return
    }
    if (!mountedRef.current) {
      setIsReadingVersions(false)
      return
    }
    // The backend redirects to Google, then back to its callback, then to the
    // frontend; a consent refusal on the way lands on the auth page, which
    // retries a start recorded here once.
    rememberGoogleSignupStart()
    window.location.href = googleStartUrl({ returnTo, signup: versions })
    // A page restored from the back-forward cache keeps this state.
    setIsReadingVersions(false)
  }

  return (
    <Button
      type="button"
      variant="outline"
      onClick={handleGoogleLogin}
      disabled={isReadingVersions}
      className={className}
    >
      {isReadingVersions ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <GoogleIcon className="h-4 w-4" />
      )}
      {variant === 'signup' ? 'Sign up with Google' : 'Continue with Google'}
    </Button>
  )
}

// Google "G" logo SVG component
function GoogleIcon({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
        fill="#4285F4"
      />
      <path
        d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
        fill="#34A853"
      />
      <path
        d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
        fill="#FBBC05"
      />
      <path
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
        fill="#EA4335"
      />
    </svg>
  )
}
