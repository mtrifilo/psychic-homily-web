'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Loader2, ClipboardList, Music, ArrowRight } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { Button } from '@/components/ui/button'
import { MyPendingEditsList } from '@/features/contributions'

/**
 * /submissions — contributor pending-edits feedback loop.
 *
 * Replaces the previous email-verification gate (PSY-600). The page now
 * renders the signed-in user's own pending entity edits — status,
 * moderator response (when rejected), and a link to the affected entity
 * — so contributors can track the lifecycle of edits they suggested via
 * EntityEditDrawer. Anonymous users redirect to /auth.
 *
 * Show submission has its own home at /shows/submit (the form itself is
 * gated on email verification there).
 */
export default function SubmissionsPage() {
  const router = useRouter()
  const { isAuthenticated, isLoading } = useAuthContext()

  // Redirect unauthenticated users to login. Mirrors the previous /submissions
  // behaviour so existing bookmarks land on the same auth flow.
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth?returnTo=%2Fsubmissions')
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

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-3xl">
        <div className="mb-8">
          <div className="flex items-center gap-3 mb-2">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
              <ClipboardList className="h-5 w-5 text-primary" />
            </div>
            <h1 className="text-2xl font-bold tracking-tight">My Submissions</h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Edits you have suggested to artists, venues, festivals, releases,
            and labels. Approved edits land on the entity; rejected ones come
            with a moderator response.
          </p>
        </div>

        <MyPendingEditsList />

        {/* Submit-a-show shortcut — preserves the discoverability of the
            previous page's main affordance now that the show form lives at
            /shows/submit. */}
        <div className="mt-8 rounded-lg border border-border/50 bg-card/50 p-4">
          <div className="flex items-center justify-between gap-4 flex-wrap">
            <div className="flex items-center gap-3 min-w-0">
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-primary/10 text-primary shrink-0">
                <Music className="h-4 w-4" />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-medium">Have a show to add?</p>
                <p className="text-xs text-muted-foreground">
                  Submit it to the calendar.
                </p>
              </div>
            </div>
            <Button asChild size="sm" variant="outline">
              <Link href="/shows/submit" className="gap-2">
                Submit a Show
                <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
