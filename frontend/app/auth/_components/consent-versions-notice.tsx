import { Loader2 } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'

/**
 * Why a sign-up was not sent: the published Terms of Service and Privacy
 * Policy versions could not be read, and a sign-up carries no others.
 */
export const CONSENT_VERSIONS_UNAVAILABLE =
  "We couldn't confirm the current Terms of Service and Privacy Policy, so your account was not created. Check your connection and try again."

export const CONSENT_RETRY_NOTICE =
  'Our Terms of Service or Privacy Policy may have just been updated. Trying again with the current versions.'

/**
 * Shown while a sign-up the backend refused for its consent versions is sent
 * once more with versions read past every cache.
 */
export function ConsentRetryNotice({ className }: { className?: string }) {
  return (
    <Alert role="status" className={className}>
      <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
      <AlertDescription>{CONSENT_RETRY_NOTICE}</AlertDescription>
    </Alert>
  )
}
