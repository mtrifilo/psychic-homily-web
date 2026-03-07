'use client'

import { Suspense, useEffect, useRef } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { useMutation } from '@tanstack/react-query'
import { Loader2, CheckCircle2, AlertCircle, BellOff } from 'lucide-react'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

function UnsubscribeContent() {
  const searchParams = useSearchParams()
  const uid = searchParams.get('uid')
  const sig = searchParams.get('sig')
  const attemptedRef = useRef(false)

  const unsubscribe = useMutation({
    mutationFn: async ({ uid, sig }: { uid: string; sig: string }) => {
      return apiRequest(API_ENDPOINTS.AUTH.UNSUBSCRIBE_SHOW_REMINDERS, {
        method: 'POST',
        body: JSON.stringify({ uid: Number(uid), sig }),
      })
    },
  })

  useEffect(() => {
    if (!uid || !sig || attemptedRef.current) return
    attemptedRef.current = true
    unsubscribe.mutate({ uid, sig })
  }, [uid, sig, unsubscribe])

  if (!uid || !sig) {
    return (
      <div className="min-h-[calc(100vh-64px)] px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card className="border-amber-500/20 bg-amber-500/5">
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/10">
                <AlertCircle className="h-6 w-6 text-amber-500" />
              </div>
              <CardTitle>Invalid Unsubscribe Link</CardTitle>
              <CardDescription>
                This unsubscribe link appears to be invalid.
              </CardDescription>
            </CardHeader>
          </Card>
        </div>
      </div>
    )
  }

  if (unsubscribe.isPending) {
    return (
      <div className="min-h-[calc(100vh-64px)] px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card>
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Loader2 className="h-6 w-6 text-primary animate-spin" />
              </div>
              <CardTitle>Unsubscribing...</CardTitle>
              <CardDescription>
                Please wait while we process your request.
              </CardDescription>
            </CardHeader>
          </Card>
        </div>
      </div>
    )
  }

  if (unsubscribe.isError) {
    return (
      <div className="min-h-[calc(100vh-64px)] px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card className="border-destructive/20 bg-destructive/5">
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
                <AlertCircle className="h-6 w-6 text-destructive" />
              </div>
              <CardTitle>Unsubscribe Failed</CardTitle>
              <CardDescription>
                We could not process your unsubscribe request.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-center">
              <p className="text-sm text-muted-foreground mb-4">
                You can disable show reminders from your settings instead.
              </p>
              <Button asChild>
                <Link href="/profile?tab=settings">Go to Settings</Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  if (unsubscribe.isSuccess) {
    return (
      <div className="min-h-[calc(100vh-64px)] px-4 py-8">
        <div className="mx-auto max-w-md">
          <Card className="border-emerald-500/20 bg-emerald-500/5">
            <CardHeader className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-emerald-500/10">
                <CheckCircle2 className="h-6 w-6 text-emerald-500" />
              </div>
              <CardTitle>Unsubscribed</CardTitle>
              <CardDescription>
                You&apos;ve been unsubscribed from show reminders.
              </CardDescription>
            </CardHeader>
            <CardContent className="text-center">
              <p className="text-sm text-muted-foreground mb-4">
                You can re-enable reminders anytime from your settings.
              </p>
              <Button asChild variant="outline">
                <Link href="/profile?tab=settings">Go to Settings</Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-md">
        <Card>
          <CardHeader className="text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
              <BellOff className="h-6 w-6 text-muted-foreground" />
            </div>
            <CardTitle>Unsubscribe</CardTitle>
            <CardDescription>Processing your request...</CardDescription>
          </CardHeader>
        </Card>
      </div>
    </div>
  )
}

function UnsubscribeLoading() {
  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
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

export default function UnsubscribeShowRemindersPage() {
  return (
    <Suspense fallback={<UnsubscribeLoading />}>
      <UnsubscribeContent />
    </Suspense>
  )
}
