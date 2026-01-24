'use client'

import { Suspense, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Loader2, CheckCircle2, AlertCircle, Mail } from 'lucide-react'
import { useVerifyMagicLink } from '@/lib/hooks/useAuth'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

function MagicLinkContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get('token')
  const { setUser } = useAuthContext()
  const verifyMagicLink = useVerifyMagicLink()
  const [hasAttempted, setHasAttempted] = useState(false)

  useEffect(() => {
    // Only attempt verification once
    if (token && !hasAttempted && !verifyMagicLink.isPending) {
      setHasAttempted(true)
      verifyMagicLink.mutate(token, {
        onSuccess: data => {
          if (data.user) {
            setUser({
              id: data.user.id,
              email: data.user.email,
              first_name: data.user.first_name,
              last_name: data.user.last_name,
              email_verified: true,
            })
          }
          // Redirect after short delay to show success message
          setTimeout(() => {
            router.push('/')
          }, 1500)
        },
      })
    }
  }, [token, hasAttempted, verifyMagicLink, setUser, router])

  // No token provided
  if (!token) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background px-4 py-12">
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
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background px-4 py-12">
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
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background px-4 py-12">
        <Card className="w-full max-w-md border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10">
              <CheckCircle2 className="h-6 w-6 text-emerald-500" />
            </div>
            <CardTitle>Welcome back!</CardTitle>
            <CardDescription>
              You've been signed in successfully. Redirecting...
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  // Error state
  return (
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background px-4 py-12">
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
    <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background px-4 py-12">
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
