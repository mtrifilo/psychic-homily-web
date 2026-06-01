'use client'

import { Suspense, useEffect, useRef } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Loader2, CheckCircle2, AlertCircle, Mail, ServerCrash } from 'lucide-react'
import { useVerifyMagicLink } from '@/features/auth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * Distinguish a genuine backend/transport failure from an expired/invalid
 * magic link.
 *
 * Background (PSY-875 → PSY-881): the backend's invalid/expired-token path
 * returns HTTP 200 + `{ success: false, error_code: "INVALID_TOKEN" }`, which
 * `useVerifyMagicLink` re-throws as an AuthError with `status: 401`. After
 * PSY-875 flipped the JWT-mint failure from a silent 200 to a real 5xx, a
 * failed mint now surfaces as an ApiError with `status >= 500`, and a network
 * outage re-throws the raw fetch error with no `status` at all.
 *
 * "Link Expired" copy (request-a-new-link CTA) is only correct for the
 * body-encoded token error — a new link cannot fix a 5xx or a dead network.
 * So treat anything that is NOT a sub-500 status (i.e. 5xx OR a status-less
 * transport throw) as a server-side failure that wants a retry CTA instead.
 */
function isServerSideFailure(error: unknown): boolean {
  const status = (error as { status?: number } | null)?.status
  return typeof status !== 'number' || status >= 500
}

function MagicLinkContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get('token')
  const { setUser } = useAuthContext()
  const verifyMagicLink = useVerifyMagicLink()
  const attemptedTokenRef = useRef<string | null>(null)
  const redirectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    // Attempt verification once per token value.
    if (!token || attemptedTokenRef.current === token) {
      return
    }
    attemptedTokenRef.current = token
    verifyMagicLink.mutate(token, {
      onSuccess: data => {
        if (data.user) {
          setUser({
            id: data.user.id,
            email: data.user.email,
            first_name: data.user.first_name,
            last_name: data.user.last_name,
            email_verified: true,
            is_admin: data.user.is_admin,
          })
        }
        // Redirect after short delay to show success message.
        redirectTimerRef.current = setTimeout(() => {
          router.push('/')
        }, 1500)
      },
    })
  }, [token, verifyMagicLink, setUser, router])

  useEffect(() => {
    return () => {
      if (redirectTimerRef.current) {
        clearTimeout(redirectTimerRef.current)
      }
    }
  }, [])

  // No token provided
  if (!token) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
        <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
              <AlertCircle className="h-6 w-6 text-destructive" />
            </div>
            <CardTitle>Invalid Link</CardTitle>
            <CardDescription>
              This magic link is invalid or incomplete. Please request a new one.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <Button onClick={() => router.push('/auth')}>
              Back to Sign In
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  // Loading state
  if (verifyMagicLink.isPending || (!verifyMagicLink.isError && !verifyMagicLink.isSuccess)) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
        <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <Loader2 className="h-6 w-6 text-primary animate-spin" />
            </div>
            <CardTitle>Signing you in...</CardTitle>
            <CardDescription>
              Please wait while we verify your magic link.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  // Success state
  if (verifyMagicLink.isSuccess) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
        <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10">
              <CheckCircle2 className="h-6 w-6 text-emerald-500" />
            </div>
            <CardTitle>Welcome back!</CardTitle>
            <CardDescription>
              You&apos;ve been signed in successfully. Redirecting...
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  // Server-side / transport failure (5xx mint failure post-PSY-875, or a
  // network outage). Requesting a new link won't help, so offer a retry of the
  // same token instead of the "Link Expired" request-new-link CTA.
  if (isServerSideFailure(verifyMagicLink.error)) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
        <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
              <ServerCrash className="h-6 w-6 text-destructive" />
            </div>
            <CardTitle>Something went wrong</CardTitle>
            <CardDescription>
              We couldn&apos;t sign you in because of a problem on our end. Your
              link is still valid — please try again in a moment.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <Button
              onClick={() => {
                // Force a fresh verification of the same token.
                attemptedTokenRef.current = null
                verifyMagicLink.reset()
              }}
            >
              Try Again
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  // Expired / invalid token (body-encoded INVALID_TOKEN, HTTP 200 + AuthError
  // status 401). A new link is the correct remedy here.
  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
            <Mail className="h-6 w-6 text-destructive" />
          </div>
          <CardTitle>Link Expired</CardTitle>
          <CardDescription>
            {verifyMagicLink.error?.message || 'This magic link has expired or is invalid. Please request a new one.'}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex justify-center">
          <Button onClick={() => router.push('/auth')}>
            Back to Sign In
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}

// Loading fallback for Suspense
function MagicLinkLoading() {
  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Loader2 className="h-6 w-6 text-primary animate-spin" />
          </div>
          <CardTitle>Loading...</CardTitle>
          <CardDescription>
            Please wait while we prepare your sign-in.
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  )
}

export default function MagicLinkPage() {
  return (
    <Suspense fallback={<MagicLinkLoading />}>
      <MagicLinkContent />
    </Suspense>
  )
}
