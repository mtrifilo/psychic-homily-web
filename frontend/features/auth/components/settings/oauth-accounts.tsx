'use client'

import { useEffect, useState } from 'react'
import { usePathname, useRouter, useSearchParams } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import {
  useOAuthAccounts,
  useStartOAuthLink,
  useUnlinkOAuthAccount,
} from '@/features/auth'
// Direct backend origin, never the Next.js /api proxy: connecting an account
// is the same full-page OAuth redirect the login button performs. See
// lib/api-base.ts (PSY-1649).
import { OAUTH_BACKEND_URL } from '@/lib/api-base'
// Subpath import, not the shared barrel: this panel is reachable from a
// browse route and the barrel pulls the whole shared surface into its chunk.
import { InlineErrorBanner } from '@/components/shared/InlineErrorBanner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Loader2,
} from 'lucide-react'

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

/**
 * How the backend reports a finished link attempt on the redirect back here.
 * The same two names are written by redirectToLinkResult in
 * backend/internal/api/handlers/auth/oauth_link.go.
 */
const OAUTH_LINK_RESULT_PARAM = 'oauth_link'
const OAUTH_LINK_ERROR_PARAM = 'oauth_link_error'

type OAuthLinkResult =
  | { status: 'connected' }
  | { status: 'error'; message: string }

/**
 * Copy for each refusal the backend can report, keyed by its error code.
 *
 * The parameter carries a CODE, never a sentence, and this map is why. This
 * banner renders into a signed-in settings page, so prose taken from the URL
 * is prose an attacker can choose: hand the user a link to
 * /profile?tab=settings&oauth_link_error=<anything> and the application
 * appears to say it. A code that is not in this map renders the generic
 * failure, so a value the attacker invents says nothing.
 *
 * Keys match the constants in backend/internal/errors/auth.go.
 */
const OAUTH_LINK_ERROR_COPY = new Map<string, string>([
  [
    'OAUTH_IDENTITY_IN_USE',
    'This provider account is already connected to another Psychic Homily account. Disconnect it there first.',
  ],
  [
    'OAUTH_PROVIDER_ALREADY_LINKED',
    'Your account is already connected to a different account from this provider. Disconnect it first, then connect this one.',
  ],
  [
    'OAUTH_LINK_EXPIRED',
    'That connection request expired. Start it again from Settings.',
  ],
  ['OAUTH_LINK_REFUSED', 'Could not connect that account.'],
  [
    'OAUTH_LINK_NOT_FROM_SETTINGS',
    'Start the connection from this page rather than from a link.',
  ],
  ['OAUTH_LINK_START_FAILED', 'Could not start the connection. Try again.'],
  [
    'USER_EXISTS',
    'An account already uses that email address. Sign in to it instead.',
  ],
  [
    'TERMS_ACCEPTANCE_REQUIRED',
    'Accept the Terms of Service and Privacy Policy first.',
  ],
])

const OAUTH_LINK_GENERIC_ERROR = 'Could not connect that account.'

/** The one value the backend sends for success. */
const OAUTH_LINK_CONNECTED = 'connected'

function readOAuthLinkResult(
  params: URLSearchParams
): OAuthLinkResult | null {
  const failed = params.get(OAUTH_LINK_ERROR_PARAM)
  if (failed) {
    // A Map, not an object literal: a plain object inherits keys like
    // __proto__ and constructor, so a lookup on an attacker-supplied value
    // returns something that is not copy at all.
    return {
      status: 'error',
      message: OAUTH_LINK_ERROR_COPY.get(failed) ?? OAUTH_LINK_GENERIC_ERROR,
    }
  }
  // Exactly the success value, not any value: otherwise a link handed to the
  // user renders a "connected" banner for a connection that never happened.
  return params.get(OAUTH_LINK_RESULT_PARAM) === OAUTH_LINK_CONNECTED
    ? { status: 'connected' }
    : null
}

function withoutOAuthLinkResult(params: URLSearchParams): string {
  const next = new URLSearchParams(params.toString())
  next.delete(OAUTH_LINK_RESULT_PARAM)
  next.delete(OAUTH_LINK_ERROR_PARAM)
  return next.toString()
}

export function OAuthAccounts() {
  const { data, isLoading, error } = useOAuthAccounts()
  const unlinkMutation = useUnlinkOAuthAccount()
  const [unlinkProvider, setUnlinkProvider] = useState<string | null>(null)
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const router = useRouter()

  // The link result arrives as a query parameter because the backend returns
  // the browser here by redirect, a fresh page load with no other channel to
  // it. Captured at mount, so the banner survives the rewrite that takes the
  // parameters back out of the URL and a reload does not replay it.
  const [linkResult] = useState<OAuthLinkResult | null>(() =>
    readOAuthLinkResult(searchParams)
  )
  useEffect(() => {
    // The rewrite is self-limiting: once the parameters are gone the stripped
    // query equals the current one and there is nothing left to replace.
    const query = withoutOAuthLinkResult(searchParams)
    if (!linkResult || query === searchParams.toString()) return

    router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false })
  }, [linkResult, searchParams, pathname, router])

  const startLink = useStartOAuthLink()
  const [startLinkFailed, setStartLinkFailed] = useState(false)

  const handleConnectGoogle = async () => {
    setStartLinkFailed(false)
    // The AUTHENTICATED link route, not /auth/login/google: the account the
    // identity attaches to comes from this session, never from the address
    // Google returns.
    //
    // The token is minted by a same-origin authenticated request first. That
    // route is a plain GET, and the auth cookie is SameSite=Lax, which a
    // browser sends on a cross-site top-level navigation, so without a token
    // only our own page can hold, any site could push a signed-in user through
    // the connect flow.
    try {
      const { token } = await startLink.mutateAsync()
      window.location.href = `${OAUTH_BACKEND_URL}/auth/link/google?t=${encodeURIComponent(token)}`
    } catch (err) {
      // Surfaced, not only reported: the click navigates away when it works,
      // so a swallowed failure looks like a button that does nothing.
      setStartLinkFailed(true)
      Sentry.captureException(err, {
        level: 'warning',
        tags: { service: 'oauth-accounts' },
        extra: { step: 'start-link' },
      })
    }
  }

  const handleUnlink = async () => {
    if (!unlinkProvider) return

    try {
      await unlinkMutation.mutateAsync(unlinkProvider)
      setUnlinkProvider(null)
    } catch (error) {
      Sentry.captureException(error, {
        level: 'warning',
        tags: { service: 'oauth-accounts' },
        extra: { provider: unlinkProvider },
      })
    }
  }

  const googleAccount = data?.accounts?.find(acc => acc.provider === 'google')

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Connected accounts</CardTitle>
        <CardDescription>
          OAuth sign-in methods linked to this account.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {/* Google Account */}
          <div className="flex items-center justify-between gap-4">
            <div className="flex min-w-0 items-center gap-3">
              <GoogleIcon className="h-5 w-5 shrink-0" />
              <div className="min-w-0">
                {googleAccount ? (
                  <>
                    <p className="text-sm font-medium">Google</p>
                    <p className="truncate font-mono text-xs text-muted-foreground">
                      {googleAccount.email || googleAccount.name || 'Connected'}
                    </p>
                  </>
                ) : (
                  <>
                    <p className="text-sm font-medium">Google</p>
                    <p className="text-xs text-muted-foreground">Not connected</p>
                  </>
                )}
              </div>
            </div>

            {isLoading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : googleAccount ? (
              <Button
                variant="link"
                size="sm"
                className="h-auto shrink-0 px-0 text-primary"
                onClick={() => setUnlinkProvider('google')}
                disabled={unlinkMutation.isPending}
              >
                {unlinkMutation.isPending && unlinkProvider === 'google' ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  'Disconnect'
                )}
              </Button>
            ) : (
              <Button
                variant="outline"
                size="sm"
                onClick={handleConnectGoogle}
              >
                Connect
              </Button>
            )}
          </div>

          {startLinkFailed && (
            <InlineErrorBanner className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>Could not start the connection. Try again.</span>
            </InlineErrorBanner>
          )}

          {/* Result of a link attempt that redirected back to this page */}
          {linkResult?.status === 'error' && (
            <InlineErrorBanner className="flex items-start gap-2">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{linkResult.message}</span>
            </InlineErrorBanner>
          )}

          {linkResult?.status === 'connected' && (
            <div role="status" className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>Account connected successfully</span>
            </div>
          )}

          {/* Error display */}
          {error && (
            <InlineErrorBanner className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>Failed to load connected accounts</span>
            </InlineErrorBanner>
          )}

          {/* Unlink error */}
          {unlinkMutation.isError && (
            <InlineErrorBanner className="flex items-center gap-2">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{unlinkMutation.error?.message || 'Failed to disconnect account'}</span>
            </InlineErrorBanner>
          )}

          {/* Success message */}
          {unlinkMutation.isSuccess && (
            <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-400">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>Account disconnected successfully</span>
            </div>
          )}
        </div>

        {/* Unlink confirmation dialog */}
        <Dialog open={!!unlinkProvider} onOpenChange={(open) => !open && setUnlinkProvider(null)}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                Disconnect Google Account?
              </DialogTitle>
              <DialogDescription>
                You will no longer be able to sign in with this Google account.
                Make sure you have another way to access your account (password or passkey).
              </DialogDescription>
            </DialogHeader>
            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                onClick={() => setUnlinkProvider(null)}
                disabled={unlinkMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleUnlink}
                disabled={unlinkMutation.isPending}
              >
                {unlinkMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  'Disconnect'
                )}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}
