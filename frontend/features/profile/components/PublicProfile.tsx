'use client'

import { useRef, useState } from 'react'
import Link from 'next/link'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Lock, Pencil } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { SectionHeader } from '@/components/shared/SectionHeader'
import { UserTierBadge } from './UserTierBadge'
import { GetStartedChecklist } from './GetStartedChecklist'
import { ProfileSections } from './ProfileSections'
import { ProfileEmptyPrompt } from './ProfileEmptyPrompt'
import { ProfileSectionAction } from './ProfileSectionAction'
import { ProfileFollowing } from './ProfileFollowing'
import { ProfileCollections } from './ProfileCollections'
import { ProfileFieldNotes } from './ProfileFieldNotes'
import { ProfileStatsSidebar } from './ProfileStatsSidebar'
import { usePublicProfile } from '@/features/auth'
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

  if (diffDays === 0) return 'today'
  if (diffDays === 1) return 'yesterday'
  if (diffDays < 7) return `${diffDays} days ago`
  if (diffDays < 14) return '1 week ago'
  if (diffDays < 30) return `${Math.floor(diffDays / 7)} weeks ago`

  return date.toLocaleDateString('en-US', {
    month: 'short',
    year: 'numeric',
  })
}

/**
 * Header Share affordance (design boards A/C): copies the profile URL with an
 * inline confirmation — no toast library, per the mutation-feedback
 * convention.
 */
function ShareButton({ username }: { username: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  // Track the reset timer so a rapid re-click extends the confirmation
  // instead of an earlier timer clipping it short.
  const resetTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const handleShare = async () => {
    try {
      await navigator.clipboard.writeText(
        `${window.location.origin}/users/${username}`
      )
      setState('copied')
    } catch {
      // Clipboard unavailable (permissions / insecure context): say so
      // inline rather than silently doing nothing.
      setState('failed')
    }
    clearTimeout(resetTimer.current)
    resetTimer.current = setTimeout(() => setState('idle'), 2000)
  }

  return (
    <button
      type="button"
      onClick={handleShare}
      className="text-sm font-semibold hover:text-primary"
      aria-label="Copy a link to this profile"
    >
      {state === 'copied' ? (
        <span className="text-primary">Copied ✓</span>
      ) : state === 'failed' ? (
        <span className="text-destructive">Copy failed</span>
      ) : (
        'Share'
      )}
    </button>
  )
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
 * (bio + custom sections, following, collections, field notes) leads in the
 * main column, while the contribution dashboard is
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

  // Fetched here for the sidebar's headline count; <ProfileCollections>
  // below shares the same query key, so this costs no extra request.
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
              <Link href="/profile?tab=privacy">
                <Pencil className="mr-1.5 size-3.5" />
                Edit profile
              </Link>
            </Button>
          )}
        </div>
      </div>
    )
  }

  // Rendered-name chain: display_name → first_name → username. display_name
  // leads, matching the backend attribution resolver (PSY-1063); the TAIL
  // deliberately differs from the resolver (which prefers username over
  // first_name) — on the profile header the handle already renders on the
  // @username line, so repeating it as the big name would be redundant.
  const displayName =
    profile.display_name || profile.first_name || profile.username
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
      {/* Identity header (board A: large avatar, display-scale name, mono
          meta line; owner gets Edit profile + Share, visitors Share only —
          the visitor Follow button is a deferred feature, PSY-1059). */}
      <header className="mb-8 border-b border-border/60 pb-6">
        <div className="flex items-start gap-5">
          <div className="order-last ml-auto flex shrink-0 items-center gap-4">
            {isOwner && (
              <Button asChild variant="outline" size="sm">
                <Link href="/profile">
                  <Pencil className="mr-1.5 size-3.5" />
                  Edit profile
                </Link>
              </Button>
            )}
            <ShareButton username={profile.username} />
          </div>
          {/* Avatar */}
          {profile.avatar_url ? (
            <img
              src={profile.avatar_url}
              alt={`${displayName}'s avatar`}
              className="h-24 w-24 rounded-full object-cover border-2 border-border"
            />
          ) : (
            <div className="h-24 w-24 rounded-full bg-muted flex items-center justify-center text-3xl font-bold text-muted-foreground border-2 border-border">
              {(displayName || '?')[0].toUpperCase()}
            </div>
          )}

          <div className="flex-1 min-w-0">
            <h1 className="text-3xl font-bold tracking-tight">{displayName}</h1>
            <div className="mt-1.5 flex items-center gap-2 flex-wrap">
              <span className="text-sm text-muted-foreground">
                @{profile.username}
              </span>
              <UserTierBadge tier={profile.user_tier} />
            </div>

            {/* Meta line (mono, board A) — location pending a backend field */}
            <p className="mt-2 font-mono text-xs text-muted-foreground">
              joined {formatDate(profile.joined_at)}
              {profile.last_active && (
                <> · active {formatLastActive(profile.last_active)}</>
              )}
            </p>
          </div>
        </div>
      </header>

      {/* Two-column body: content leads, stats demoted to the sidebar */}
      <div className="flex flex-col gap-10 lg:flex-row">
        <div className="flex-1 min-w-0 space-y-8">
          {/* Bio — the primary content slot */}
          {(hasBio || hasSections || isOwner) && (
            <section aria-label="Bio">
              <SectionHeader
                title="Bio"
                as="h2"
                size="md"
                variant="title"
                action={
                  isOwner ? (
                    <ProfileSectionAction
                      label="Edit"
                      href="/profile"
                      ariaLabel="Edit your bio"
                    />
                  ) : undefined
                }
              />
              {hasBio ? (
                /* bio_html is server-sanitized (goldmark + bluemonday),
                   matching profile sections; fall back to raw bio when absent
                   (e.g. empty bio omits bio_html). */
                profile.bio_html ? (
                  <div
                    className="mt-2 prose prose-sm dark:prose-invert max-w-none"
                    dangerouslySetInnerHTML={{ __html: profile.bio_html }}
                  />
                ) : (
                  <p className="mt-2 text-sm leading-relaxed whitespace-pre-line">
                    {profile.bio}
                  </p>
                )
              ) : isOwner ? (
                <ProfileEmptyPrompt
                  message="Add a short bio so people know who you are — the scenes you haunt, what you're into."
                  ctaLabel="Add bio"
                  ctaHref="/profile"
                />
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
              <ProfileFollowing username={username} isOwner={isOwner} />

              <ProfileCollections username={username} isOwner={isOwner} />

              <ProfileFieldNotes username={username} />

              {/* Recent activity (contribution timeline) was removed per the
                  2026-06-10 source-of-truth decision — it appears on no
                  design board; the timeline stays reachable from the
                  contribution surfaces. */}
            </>
          )}

          {/* Visitor-facing empty state: public profile with nothing on it.
              Requires stats to be VISIBLE and zero — when stats are hidden we
              can't know whether the (self-fetching) list sections rendered
              content, so we don't claim emptiness. */}
          {!isOwner &&
            isNewProfile &&
            profile.stats?.total_contributions === 0 && (
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

        {/* order-first: on mobile the stats render as a compact strip ABOVE
            the main content (design board D); on lg the rail sits right. */}
        <aside className="order-first w-full shrink-0 lg:order-none lg:w-80">
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
