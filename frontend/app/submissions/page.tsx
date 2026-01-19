'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Loader2, Music, AlertCircle, Mail, Settings, ArrowRight } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'

function EmailVerificationRequired() {
  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-lg">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/10">
            <Mail className="h-6 w-6 text-amber-500" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Email Verification Required</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Please verify your email to submit shows
          </p>
        </div>

        {/* Info Card */}
        <Card className="border-amber-500/20 bg-amber-500/5">
          <CardHeader>
            <div className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-amber-500" />
              <CardTitle className="text-lg">Why verify your email?</CardTitle>
            </div>
            <CardDescription>
              To maintain the quality of our community calendar, we require email verification before you can submit shows.
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
              <Button asChild className="w-full gap-2">
                <Link href="/collection?tab=settings">
                  <Settings className="h-4 w-4" />
                  Go to Settings to Verify Email
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

export default function SubmissionsPage() {
  const router = useRouter()
  const { isAuthenticated, isLoading, user } = useAuthContext()

  // Redirect unauthenticated users to login
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth')
    }
  }, [isAuthenticated, isLoading, router])

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Don't render if not authenticated (will redirect)
  if (!isAuthenticated) {
    return null
  }

  // Check if user can submit shows (admin or verified email)
  const canSubmit = user?.is_admin || user?.email_verified

  // Show verification required screen for non-admin users with unverified emails
  if (!canSubmit) {
    return <EmailVerificationRequired />
  }

  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-lg">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Music className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Submit a Show</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Add an upcoming show to the Arizona music calendar
          </p>
          {user && (
            <p className="mt-1 text-xs text-muted-foreground">
              Submitting as {user.email}
            </p>
          )}
        </div>

        {/* Form Card */}
        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardContent className="pt-6">
            <ShowForm mode="create" />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
