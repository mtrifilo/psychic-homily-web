'use client'

import { Suspense, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { Loader2, CheckCircle2, AlertCircle, Mail, ArrowRight } from 'lucide-react'
import { useConfirmVerification } from '@/lib/hooks/useAuth'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

function VerifyEmailContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get('token')
  const confirmVerification = useConfirmVerification()
  const [hasAttempted, setHasAttempted] = useState(false)

  // Automatically verify when token is present
  useEffect(() => {
    if (token && !hasAttempted) {
      setHasAttempted(true)
      confirmVerification.mutate(token)
    }
  }, [token, hasAttempted, confirmVerification])

  // No token provided
  if (!token) {
    return (
      <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card className="border-amber-500/20 bg-amber-500/5">
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/10">
                <AlertCircle className="h-6 w-6 text-amber-500" />
              </div>
              <CardTitle>Invalid Verification Link</CardTitle>
              <CardDescription>
                This verification link appears to be invalid or expired.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-center">
              <p className="text-sm text-muted-foreground mb-4">
                Please request a new verification email from your settings.
              </p>
              <Button asChild>
                <Link href="/collection?tab=settings">
                  Go to Settings
                </Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  // Loading state
  if (confirmVerification.isPending) {
    return (
      <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card>
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Loader2 className="h-6 w-6 text-primary animate-spin" />
              </div>
              <CardTitle>Verifying Your Email</CardTitle>
              <CardDescription>
                Please wait while we verify your email address...
              </CardDescription>
            </CardHeader>
          </Card>
        </div>
      </div>
    )
  }

  // Error state
  if (confirmVerification.isError) {
    return (
      <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card className="border-destructive/20 bg-destructive/5">
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
                <AlertCircle className="h-6 w-6 text-destructive" />
              </div>
              <CardTitle>Verification Failed</CardTitle>
              <CardDescription>
                {confirmVerification.error?.message || 'We could not verify your email address.'}
              </CardDescription>
            </CardHeader>
            <CardContent className="text-center space-y-4">
              <p className="text-sm text-muted-foreground">
                The verification link may have expired or already been used.
              </p>
              <Button asChild>
                <Link href="/collection?tab=settings">
                  Request New Verification Email
                </Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  // Success state
  if (confirmVerification.isSuccess) {
    return (
      <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card className="border-emerald-500/20 bg-emerald-500/5">
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10">
                <CheckCircle2 className="h-6 w-6 text-emerald-500" />
              </div>
              <CardTitle>Email Verified!</CardTitle>
              <CardDescription>
                Your email address has been successfully verified.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-center space-y-4">
              <p className="text-sm text-muted-foreground">
                You can now submit shows to the Arizona music calendar.
              </p>
              <div className="flex flex-col gap-2">
                <Button asChild className="gap-2">
                  <Link href="/submissions">
                    <Mail className="h-4 w-4" />
                    Submit a Show
                    <ArrowRight className="h-4 w-4" />
                  </Link>
                </Button>
                <Button asChild variant="outline">
                  <Link href="/collection">
                    Go to My Collection
                  </Link>
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  // Default/initial state (shouldn't normally be visible)
  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-md">
        <Card>
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
              <Mail className="h-6 w-6 text-muted-foreground" />
            </div>
            <CardTitle>Email Verification</CardTitle>
            <CardDescription>
              Processing your verification request...
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    </div>
  )
}

function VerifyEmailLoading() {
  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-md">
        <Card>
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <Loader2 className="h-6 w-6 text-primary animate-spin" />
            </div>
            <CardTitle>Loading...</CardTitle>
          </CardHeader>
        </Card>
      </div>
    </div>
  )
}

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={<VerifyEmailLoading />}>
      <VerifyEmailContent />
    </Suspense>
  )
}
