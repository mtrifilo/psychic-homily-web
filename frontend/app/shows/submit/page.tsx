'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import * as Sentry from '@sentry/nextjs'
import {
  Loader2,
  Music,
  AlertCircle,
  CheckCircle2,
  Mail,
  Settings,
  ArrowRight,
} from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useSendVerificationEmail } from '@/features/auth'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { AIFormFiller, ShowForm } from '@/features/shows'
import type { ExtractedShowData } from '@/lib/types/extraction'

/**
 * Gate shown to a signed-in but unverified user.
 *
 * The resend button is here, at the point of blockage, rather than only in
 * Settings (PSY-1871). Signup now emails the link, so the common case is "it
 * expired" or "I never got it", and sending the user two hops away to Settings
 * to fix that loses most of them.
 */
function EmailVerificationRequired() {
  const sendVerificationEmail = useSendVerificationEmail()
  const [emailSent, setEmailSent] = useState(false)

  const handleResend = async () => {
    try {
      await sendVerificationEmail.mutateAsync()
      setEmailSent(true)
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'shows_submit', error_type: 'verification_email' },
      })
    }
  }

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-lg">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/10">
            <Mail className="h-6 w-6 text-amber-500" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">
            Email Verification Required
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Please verify your email to submit shows
          </p>
        </div>

        <Card className="border-amber-500/20 bg-amber-500/5">
          <CardHeader>
            <div className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-amber-500" />
              <CardTitle className="text-lg">Why verify your email?</CardTitle>
            </div>
            <CardDescription>
              To maintain the quality of our community calendar, we require
              email verification before you can submit shows.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Verifying your email helps us:
            </p>
            <ul className="text-sm text-muted-foreground list-disc list-inside space-y-1">
              <li>Ensure show submissions come from real users</li>
              <li>Contact you if there are questions about your submission</li>
              <li>Keep the calendar accurate and spam-free</li>
            </ul>

            <div className="pt-4 space-y-3">
              {/*
                Deliberately not "we emailed you when you signed up": every
                account created before this shipped never got one, and that
                backlog is exactly the population landing on this gate. A send
                can also be skipped or fail silently by design.
              */}
              <p className="text-sm text-muted-foreground">
                Send yourself a verification link. If you already have one and
                it has expired, this replaces it.
              </p>

              {emailSent && sendVerificationEmail.isSuccess ? (
                <div className="flex items-center gap-2 text-sm text-success-foreground">
                  <CheckCircle2 className="h-4 w-4 shrink-0" />
                  <span>Verification email sent! Check your inbox.</span>
                </div>
              ) : (
                <Button
                  onClick={handleResend}
                  disabled={sendVerificationEmail.isPending}
                  className="w-full gap-2"
                >
                  {sendVerificationEmail.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Mail className="h-4 w-4" />
                  )}
                  {sendVerificationEmail.isPending
                    ? 'Sending...'
                    : 'Send verification email'}
                </Button>
              )}

              {sendVerificationEmail.isError && (
                <div
                  role="alert"
                  className="flex items-center gap-2 text-sm text-destructive"
                >
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  <span>
                    {sendVerificationEmail.error?.message ||
                      'Failed to send verification email. Please try again.'}
                  </span>
                </div>
              )}

              <Button asChild variant="outline" className="w-full gap-2">
                <Link href="/profile?tab=settings">
                  <Settings className="h-4 w-4" />
                  Manage email in Settings
                  <ArrowRight className="h-4 w-4" />
                </Link>
              </Button>
              <p className="text-xs text-center text-muted-foreground">
                After verifying, come back here to submit your show
              </p>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

/**
 * /shows/submit — show-submission form.
 *
 * Moved here from /submissions in PSY-600 to free up that path for the
 * contributor pending-edits surface. Behaviour preserved:
 *   - anonymous → redirect to login (returnTo back here)
 *   - authenticated but unverified email → "Verify email" gate
 *   - authenticated + verified (or admin) → form
 */
export default function SubmitShowPage() {
  const router = useRouter()
  const { isAuthenticated, isLoading, user } = useAuthContext()

  const [extractedData, setExtractedData] = useState<
    ExtractedShowData | undefined
  >()
  // Bumped on each extraction so <ShowForm key={...}> remounts and re-seeds
  // its defaultValues from the new extraction (PSY-795 — replaces the prior
  // prop-derived useEffect inside ShowForm).
  const [extractionVersion, setExtractionVersion] = useState(0)

  const handleExtracted = (data: ExtractedShowData) => {
    setExtractedData(data)
    setExtractionVersion(v => v + 1)
  }

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth?returnTo=%2Fshows%2Fsubmit')
    }
  }, [isAuthenticated, isLoading, router])

  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  const canSubmit = user?.is_admin || user?.email_verified

  if (!canSubmit) {
    return <EmailVerificationRequired />
  }

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-lg">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Music className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Submit a Show</h1>
          {user && (
            <p className="mt-1 text-xs text-muted-foreground">
              Submitting as {user.email}
            </p>
          )}
        </div>

        <AIFormFiller onExtracted={handleExtracted} />

        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardContent className="pt-4">
            <ShowForm
              key={extractionVersion}
              mode="create"
              initialExtraction={extractedData}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
