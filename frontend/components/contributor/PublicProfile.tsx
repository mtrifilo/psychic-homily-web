'use client'

import Link from 'next/link'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Lock, CalendarDays, Clock, Pencil } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { UserTierBadge } from './UserTierBadge'
import { ContributionTimeline } from './ContributionTimeline'
import { GetStartedChecklist } from './GetStartedChecklist'
import { ProfileSections } from './ProfileSections'
import { ProfileFollowing } from './ProfileFollowing'
import { ProfileAttendedShows } from './ProfileAttendedShows'
import { ProfileFieldNotes } from './ProfileFieldNotes'
import { ProfileStatsSidebar } from './ProfileStatsSidebar'
import {
  usePublicProfile,
  usePublicContributions,
} from '@/features/auth'
import { UserCollections } from '@/features/collections'
import { useUserPublicCollections } from '@/features/collections'

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    month: 'long',
    year: 'numeric',
  })
}

function formatLastActive(dateString: string): string {
  const date = new Date(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays} days ago`
  if (diffDays < 30) return `${Math.floor(diffDays / 7)} weeks ago`

  return date.toLocaleDateString('en-US', {
    month: 'short',
    year: 'numeric',
  })
}

function ProfileSkeleton() {
  return (
    <div className="container max-w-6xl mx-auto px-4 py-12">
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Skeleton className="h-16 w-16 rounded-full" />
          <div className="space-y-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-32" />
          </div>
        </div>
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    </div>
  )
}

interface PublicProfileProps {
  username: string
}

/**
 * The public profile as a content-first identity hub (PSY-1045).
 *
 * Layout: identity header, then a two-column body — the user's CONTENT
 * (bio + custom sections, following, collections, concert diary, field
 * notes) leads in the main column, while the contribution dashboard is
 * demoted to a compact, expandable sidebar card. This inverts the previous
 * stats-first layout per the Gazelle "identity hub, not a stats dump"
 * direction (docs/features/profile-redesign.md).
 */
export function PublicProfile({ username }: PublicProfileProps) {
  const { user } = useAuthContext()

  const {
    data: profile,
    isLoading,
    error,
  } = usePublicProfile(username)

  const {
    data: contributionsData,
  } = usePublicContributions(username, { limit: 10 })

  // Fetched here for the sidebar's headline count; <UserCollections> below
  // shares the same query key, so this costs no extra request.
  const { data: collectionsData } = useUserPublicCollections(username)

  // The viewer is the profile owner when their logged-in username matches the
  // profile being viewed. Compared case-insensitively only to stay robust to
  // URL casing — it gates ONLY a convenience "Edit profile" affordance and
  // exposes no owner-only data, so a spurious match would leak nothing (the
  // /profile edit route is itself auth-gated and scoped to the logged-in user
  // server-side; this is a UI shortcut, not an authorization boundary). PSY-1025.
  const isOwner = Boolean(
    user?.username &&
      user.username.toLowerCase() === username.toLowerCase()
  )

  if (isLoading) {
    return <ProfileSkeleton />
  }

  if (error) {
    const apiError = error as { status?: number }
    if (apiError.status === 404) {
      return (
        <div className="container max-w-6xl mx-auto px-4 py-12">
          <div className="text-center py-16">
            <h1 className="text-2xl font-bold mb-2">User Not Found</h1>
            <p className="text-muted-foreground">
              The user &ldquo;{username}&rdquo; could not be found.
            </p>
          </div>
        </div>
      )
    }

    return (
      <div className="container max-w-6xl mx-auto px-4 py-12">
        <div className="text-center py-16">
          <h1 className="text-2xl font-bold mb-2">Error</h1>
          <p className="text-muted-foreground">
            Failed to load profile. Please try again later.
          </p>
        </div>
      </div>
    )
  }

  if (!profile) {
    return null
  }

  // Private profile. The backend returns this page to the owner even when
  // private (with full owner data), so without the isOwner branch an owner
  // who clicks "Profile" (which now routes here) would hit a dead-end wall
  // with no way to reach settings. Give the owner their own copy + the Edit
  // affordance so they can change visibility; visitors keep the plain wall.
  // PSY-1025.
  if (profile.profile_visibility === 'private') {
    return (
      <div className="container max-w-6xl mx-auto px-4 py-12">
        <div className="text-center py-16">
          <div className="inline-flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Lock className="h-8 w-8 text-muted-foreground" />
          </div>
          <h1 className="text-2xl font-bold mb-2">
            {isOwner ? 'Your Profile Is Private' : 'Private Profile'}
          </h1>
          <p className="text-muted-foreground">
            {isOwner
              ? 'Only you can see this page. Edit your profile to make it public.'
              : "This user's profile is set to private."}
          </p>
          {isOwner && (
            <Button asChild variant="outline" size="sm" className="mt-6">
              <Link href="/profile">
                <Pencil className="mr-1.5 size-3.5" />
                Edit profile
              </Link>
            </Button>
          )}
        </div>
      </div>
    )
  }

  const displayName = profile.first_name || profile.username
  const contributions = contributionsData?.contributions || []
  const hasBio = Boolean(profile.bio)
  const visibleSections = (profile.sections ?? []).filter(s => s.is_visible)
  const hasSections = visibleSections.length > 0
  const collectionsTotal = collectionsData?.total
  const hasCollections = (collectionsTotal ?? 0) > 0

  // A brand-new owner profile: no prose, no contributions yet. Replace the
  // empty sections with the onboarding checklist (design board B).
  const isNewProfile =
    !hasBio &&
    !hasSections &&
    !hasCollections &&
    (profile.stats?.total_contributions ?? 0) === 0
  const showGetStarted = isOwner && isNewProfile

  return (
    <div className="container max-w-6xl mx-auto px-4 py-10">
      {/* Identity header */}
      <header className="mb-8 border-b border-border/60 pb-6">
        <div className="flex items-start gap-4">
          {isOwner && (
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
          )}
          {/* Avatar */}
          {profile.avatar_url ? (
            <img
              src={profile.avatar_url}
              alt={`${displayName}'s avatar`}
              className="h-16 w-16 rounded-full object-cover border-2 border-border"
            />
          ) : (
            <div className="h-16 w-16 rounded-full bg-muted flex items-center justify-center text-2xl font-bold text-muted-foreground border-2 border-border">
              {(displayName || '?')[0].toUpperCase()}
            </div>
          )}

          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-2xl font-bold">{displayName}</h1>
              <UserTierBadge tier={profile.user_tier} />
            </div>
            <p className="text-sm text-muted-foreground mt-0.5">
              @{profile.username}
            </p>

            {/* Meta info */}
            <div className="flex items-center gap-4 mt-2 text-xs text-muted-foreground">
              <span className="flex items-center gap-1">
                <CalendarDays className="h-3.5 w-3.5" />
                Joined {formatDate(profile.joined_at)}
              </span>
              {profile.last_active && (
                <span className="flex items-center gap-1">
                  <Clock className="h-3.5 w-3.5" />
                  Active {formatLastActive(profile.last_active)}
                </span>
              )}
            </div>
          </div>
        </div>
      </header>

      {/* Two-column body: content leads, stats demoted to the sidebar */}
      <div className="flex flex-col gap-10 lg:flex-row">
        <div className="flex-1 min-w-0 space-y-8">
          {/* Bio — the primary content slot */}
          {(hasBio || hasSections || isOwner) && (
            <section aria-label="Bio">
              <SectionHeader title="Bio" as="h2" size="md" />
              {hasBio ? (
                <p className="mt-2 text-sm leading-relaxed whitespace-pre-line">
                  {profile.bio}
                </p>
              ) : isOwner ? (
                <div className="mt-2 flex items-center justify-between gap-3 rounded-md border border-dashed border-border bg-muted/20 px-4 py-3">
                  <p className="text-sm text-muted-foreground">
                    Add a short bio so people know who you are — the scenes
                    you haunt, what you&apos;re into.
                  </p>
                  <Button asChild variant="outline" size="sm" className="shrink-0">
                    <Link href="/profile">Add bio</Link>
                  </Button>
                </div>
              ) : null}
              {hasSections && (
                <div className="mt-4">
                  <ProfileSections sections={visibleSections} />
                </div>
              )}
            </section>
          )}

          {showGetStarted ? (
            <GetStartedChecklist />
          ) : (
            <>
              <ProfileFollowing username={username} />

              {(hasCollections || isOwner) && (
                <section aria-label="Collections">
                  <SectionHeader title="Collections" as="h2" size="md" />
                  <div className="mt-3">
                    <UserCollections username={username} />
                  </div>
                </section>
              )}

              <ProfileAttendedShows username={username} />
              <ProfileFieldNotes username={username} />

              {/* Recent contributions — kept as the trailing section so the
                  knowledge-graph work stays visible without leading the page. */}
              {contributions.length > 0 && (
                <section aria-label="Recent activity">
                  <SectionHeader title="Recent activity" as="h2" size="md" />
                  <Card className="mt-3 bg-muted/30 border-border/50">
                    <CardContent className="p-2">
                      <ContributionTimeline contributions={contributions} />
                    </CardContent>
                  </Card>
                </section>
              )}
            </>
          )}

          {/* Visitor-facing empty state: public profile with nothing on it.
              Requires stats to be VISIBLE and zero — when stats are hidden we
              can't know whether the (self-fetching) list sections rendered
              content, so we don't claim emptiness. */}
          {!isOwner &&
            isNewProfile &&
            profile.stats?.total_contributions === 0 &&
            contributions.length === 0 && (
              <Card className="bg-muted/30 border-border/50">
                <CardContent className="p-8 text-center">
                  <p className="text-sm text-muted-foreground">
                    This user hasn&apos;t added any content to their profile
                    yet.
                  </p>
                </CardContent>
              </Card>
            )}
        </div>

        <aside className="w-full lg:w-80 shrink-0">
          <ProfileStatsSidebar
            username={username}
            stats={profile.stats}
            statsCount={profile.stats_count}
            collectionsTotal={collectionsTotal}
            isOwner={isOwner}
          />
        </aside>
      </div>
    </div>
  )
}
