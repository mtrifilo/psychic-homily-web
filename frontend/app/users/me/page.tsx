'use client'

import { useEffect } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Loader2, Flag, Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SectionHeader } from '@/components/shared/SectionHeader'
import {
  GetStartedChecklist,
  ProfileStatsSidebar,
  UserTierBadge,
} from '@/features/profile'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useOwnContributorProfile } from '@/features/auth'
import { useMyCollections } from '@/features/collections'

function CenteredSpinner() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

/**
 * /users/me — the self view of the public profile (PSY-1045).
 *
 * Static segment, so it wins over the dynamic /users/[username] route and is
 * NOT intercepted by the entity proxy (which only matches entity prefixes).
 *
 * - Authed with a username → redirect to the real public profile.
 * - Authed without a username (OAuth-only accounts) → the claim-username
 *   state (design board B): same profile-shaped page with a claim banner and
 *   the Get-started checklist, so the user sees the profile experience
 *   before their URL exists.
 * - Unauthenticated → /auth.
 */
export default function SelfProfilePage() {
  const router = useRouter()
  const { user, isAuthenticated, isLoading } = useAuthContext()
  const { data: profile } = useOwnContributorProfile()
  // Own-collections total for the stats card. The public count is
  // username-keyed and can't resolve here; the self view counts the user's
  // own collections instead (includes private ones — it's their own card).
  const { data: myCollections } = useMyCollections()

  useEffect(() => {
    if (isLoading) return
    if (!isAuthenticated) {
      router.replace('/auth')
      return
    }
    if (user?.username) {
      router.replace(`/users/${user.username}`)
    }
  }, [isLoading, isAuthenticated, user?.username, router])

  // While auth state resolves, or while a redirect above is in flight.
  if (isLoading || !isAuthenticated || user?.username) {
    return <CenteredSpinner />
  }

  // display_name leads across both sources (PSY-1063); username is absent
  // from this chain by construction — this page only renders for accounts
  // with no username (the claim state).
  const displayName =
    user?.display_name ||
    profile?.display_name ||
    user?.first_name ||
    profile?.first_name ||
    user?.email ||
    'You'

  return (
    <div className="container max-w-6xl mx-auto px-4 py-10">
      {/* Identity header — claim state (no username yet) */}
      <header className="mb-8 border-b border-border/60 pb-6">
        <div className="flex items-start gap-4">
          <Button
            asChild
            variant="outline"
            size="sm"
            className="order-last ml-auto shrink-0"
          >
            <Link href="/profile">
              <Pencil className="mr-1.5 size-3.5" />
              Edit profile
            </Link>
          </Button>
          <div className="h-16 w-16 rounded-full bg-muted flex items-center justify-center text-2xl font-bold text-muted-foreground border-2 border-border">
            {displayName[0]?.toUpperCase() ?? '?'}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-2xl font-bold">{displayName}</h1>
              {profile?.user_tier && <UserTierBadge tier={profile.user_tier} />}
            </div>
            <p className="text-sm text-muted-foreground mt-0.5">
              no username yet
            </p>
          </div>
        </div>
      </header>

      {/* Two-column body mirroring the public profile (design board B):
          onboarding content leads, the zeroed stats card sits in the rail. */}
      <div className="flex flex-col gap-10 lg:flex-row">
        <div className="min-w-0 flex-1 space-y-8">
          {/* Claim-username banner */}
          <div className="flex items-center gap-4 rounded-md border border-border bg-muted/40 px-4 py-3.5">
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-1.5 text-sm font-semibold">
                <Flag className="h-3.5 w-3.5 text-primary" aria-hidden />
                Claim your username
              </p>
              <p className="mt-0.5 text-sm text-muted-foreground">
                Your public profile lives at /users/&lt;username&gt; — pick a
                handle to make it shareable.
              </p>
            </div>
            <Button asChild size="sm" className="shrink-0">
              <Link href="/profile">Set username</Link>
            </Button>
          </div>

          <section aria-label="Bio">
            <SectionHeader title="Bio" as="h2" size="md" />
            <div className="mt-2 flex items-center justify-between gap-3 rounded-md border border-dashed border-border bg-muted/20 px-4 py-3">
              <p className="text-sm text-muted-foreground">
                Add a short bio so people know who you are — the scenes you
                haunt, what you&apos;re into.
              </p>
              <Button asChild variant="outline" size="sm" className="shrink-0">
                <Link href="/profile">Add bio</Link>
              </Button>
            </div>
          </section>

          <GetStartedChecklist />
        </div>

        <aside className="order-first w-full shrink-0 lg:order-none lg:w-80">
          {/* No username yet, so the expanded panel's per-username endpoints
              can't resolve — render the card non-expandable (board B shows the
              zero state with the onboarding hint instead of the expander). */}
          <ProfileStatsSidebar
            username=""
            stats={profile?.stats}
            collectionsTotal={myCollections?.total}
            isOwner
            expandable={false}
          />
        </aside>
      </div>
    </div>
  )
}
