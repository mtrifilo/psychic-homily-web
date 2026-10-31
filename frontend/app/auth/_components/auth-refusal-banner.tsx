import type { ReactNode } from 'react'
import { AlertCircle, Info } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { AccountRecoverableMessage } from '@/app/auth/_components/account-recoverable-message'
import { AUTH_ERROR_CODE_PARAM, AUTH_ERROR_PARAM } from '@/lib/auth-href'
import { AuthErrorCode } from '@/lib/errors'
import {
  ACCOUNT_UNAVAILABLE_MESSAGE,
  AGE_CONFIRMATION_REQUIRED_MESSAGE,
  AUTH_RATE_LIMITED_MESSAGE,
  AUTH_REFUSAL_GENERIC_MESSAGE,
  OAUTH_CANCELLED_MESSAGE,
  OAUTH_LINK_REFUSED_MESSAGE,
  TERMS_ACCEPTANCE_REQUIRED_MESSAGE,
  USER_EXISTS_MESSAGE,
  VALIDATION_FAILED_MESSAGE,
} from '@/lib/errors/authRefusalCopy'

/**
 * What the auth page says above its card for an arrival's refusal. Neutral
 * for an outcome the person chose; destructive for everything else.
 */
export type AuthRefusalBanner = {
  tone: 'neutral' | 'destructive'
  message: ReactNode
}

const destructive = (message: ReactNode): AuthRefusalBanner => ({
  tone: 'destructive',
  message,
})

const GENERIC_BANNER = destructive(AUTH_REFUSAL_GENERIC_MESSAGE)

/**
 * The banner for each code an OAuth redirect hands the auth page. `null` is a
 * code the page answers some other way: the collision state for a pending
 * Google link.
 *
 * A Map, not an object literal: a plain object inherits keys like __proto__
 * and constructor, so a lookup on a value taken from the URL would return
 * something that is not a banner.
 */
const BANNER_BY_CODE = new Map<string, AuthRefusalBanner | null>([
  [AuthErrorCode.OAUTH_LINK_PENDING, null],
  [AuthErrorCode.OAUTH_CANCELLED, { tone: 'neutral', message: OAUTH_CANCELLED_MESSAGE }],
  [AuthErrorCode.RATE_LIMITED, destructive(AUTH_RATE_LIMITED_MESSAGE)],
  [AuthErrorCode.AGE_CONFIRMATION_REQUIRED, destructive(AGE_CONFIRMATION_REQUIRED_MESSAGE)],
  [AuthErrorCode.ACCOUNT_RECOVERABLE, destructive(<AccountRecoverableMessage />)],
  [AuthErrorCode.ACCOUNT_INACTIVE, destructive(ACCOUNT_UNAVAILABLE_MESSAGE)],
  [AuthErrorCode.OAUTH_LINK_REFUSED, destructive(OAUTH_LINK_REFUSED_MESSAGE)],
  [AuthErrorCode.VALIDATION_FAILED, destructive(VALIDATION_FAILED_MESSAGE)],
  [AuthErrorCode.USER_EXISTS, destructive(USER_EXISTS_MESSAGE)],
  [AuthErrorCode.TERMS_ACCEPTANCE_REQUIRED, destructive(TERMS_ACCEPTANCE_REQUIRED_MESSAGE)],
])

/**
 * The banner for an auth-page arrival, decided by its `error_code` alone.
 *
 * A known code gets its own line. An unknown code, or `error` text with no
 * known code, gets the generic line: the text is never rendered, because
 * anyone can put text in a link. An arrival with neither (a plain visit, a
 * re-authentication, a closed-account `reason`) gets no banner.
 */
export function authRefusalBannerFor(searchParams: {
  get: (name: string) => string | null
  has: (name: string) => boolean
}): AuthRefusalBanner | null {
  const code = searchParams.get(AUTH_ERROR_CODE_PARAM)
  if (code !== null) {
    const banner = BANNER_BY_CODE.get(code)
    return banner === undefined ? GENERIC_BANNER : banner
  }
  return searchParams.has(AUTH_ERROR_PARAM) ? GENERIC_BANNER : null
}

export function AuthRefusalAlert({ banner }: { banner: AuthRefusalBanner }) {
  const neutral = banner.tone === 'neutral'
  return (
    <Alert variant={neutral ? 'default' : 'destructive'} className="mb-6">
      {neutral ? <Info className="h-4 w-4" /> : <AlertCircle className="h-4 w-4" />}
      <AlertDescription>{banner.message}</AlertDescription>
    </Alert>
  )
}
