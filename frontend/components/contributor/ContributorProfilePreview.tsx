'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Clock, TrendingUp, Award } from 'lucide-react'
import { UserTierBadge } from './UserTierBadge'
import { ContributionStatsGrid } from './ContributionStatsGrid'
import { ContributionTimeline } from './ContributionTimeline'
import {
  useOwnContributorProfile,
  useOwnContributions,
} from '@/features/auth'
import type { ContributionStats } from '@/features/auth'

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    month: 'long',
    year: 'numeric',
  })
}

function ProfilePreviewSkeleton() {
  return (
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
  )
}

export function ContributorProfilePreview() {
  const { data: profile, isLoading } = useOwnContributorProfile()
  const { data: contributionsData } = useOwnContributions({ limit: 5 })

  if (isLoading) {
    return <ProfilePreviewSkeleton />
  }

  if (!profile) {
    return (
      <Card>
        <CardContent className="p-6 text-center">
          <p className="text-sm text-muted-foreground">
            Unable to load your contributor profile.
          </p>
        </CardContent>
      </Card>
    )
  }

  // display_name leads (PSY-1063); tail mirrors PublicProfile's header chain
  // (first_name over username — the handle renders separately), see the
  // rationale comment in PublicProfile.tsx.
  const displayName =
    profile.display_name || profile.first_name || profile.username
  const contributions = contributionsData?.contributions || []
  const isPublic = profile.profile_visibility === 'public'

  return (
    <div className="space-y-6">
      {/* Profile card (design board H): identity only — no card heading, no
          View-Public-Profile button (the page header carries that link since
          PSY-1054); profile_visibility renders as the corner pill. */}
      <Card>
        <CardContent className="p-5">
          <div className="flex items-start gap-4">
            {/* Avatar */}
            {profile.avatar_url ? (
              <img
                src={profile.avatar_url}
                alt={`${displayName}'s avatar`}
                className="h-14 w-14 rounded-full object-cover border-2 border-border"
              />
            ) : (
              <div className="h-14 w-14 rounded-full bg-muted flex items-center justify-center text-xl font-bold text-muted-foreground border-2 border-border">
                {(displayName || '?')[0].toUpperCase()}
              </div>
            )}

            <div className="min-w-0 flex-1">
              <h2 className="text-lg font-semibold">{displayName}</h2>
              <div className="mt-0.5 flex items-center gap-2 flex-wrap">
                {profile.username && (
                  <span className="text-sm text-muted-foreground">
                    @{profile.username}
                  </span>
                )}
                <UserTierBadge tier={profile.user_tier} />
              </div>
              <p className="mt-2 font-mono text-xs text-muted-foreground">
                Joined {formatDate(profile.joined_at)}
              </p>
            </div>

            <span
              className={`inline-flex shrink-0 items-center gap-1.5 rounded-sm border border-border px-2 py-0.5 font-mono text-xs ${
                isPublic ? 'text-foreground' : 'text-pending-foreground'
              }`}
              aria-label={
                isPublic ? 'Profile is public' : 'Profile is private'
              }
            >
              <span aria-hidden className="text-[8px] leading-none">
                ●
              </span>
              {isPublic ? 'Public' : 'Private'}
            </span>
          </div>
        </CardContent>
      </Card>

      {/* Your Impact */}
      {profile.stats && profile.stats.total_contributions > 0 && (
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <TrendingUp className="h-5 w-5 text-muted-foreground" />
              <CardTitle className="text-lg">Your Impact</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Summary sentence */}
            <p className="text-sm text-muted-foreground">
              {buildImpactSummary(profile.stats)}
            </p>

            {/* Stats grid */}
            <ContributionStatsGrid stats={profile.stats} />
          </CardContent>
        </Card>
      )}

      {/* Recent Contributions */}
      {contributions.length > 0 && (
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center gap-2">
              <Clock className="h-5 w-5 text-muted-foreground" />
              <CardTitle className="text-lg">Recent Activity</CardTitle>
            </div>
          </CardHeader>
          <CardContent className="p-2">
            <ContributionTimeline contributions={contributions} />
          </CardContent>
        </Card>
      )}

      {/* Empty state */}
      {(!profile.stats || profile.stats.total_contributions === 0) && (
        <Card className="bg-muted/30 border-border/50 border-dashed">
          <CardContent className="p-8 text-center space-y-3">
            <Award className="h-10 w-10 text-muted-foreground/40 mx-auto" />
            <div>
              <p className="text-sm font-medium">Start Contributing</p>
              <p className="text-xs text-muted-foreground mt-1">
                Submit shows, edit venues, and help build the music knowledge graph.
                Your contributions will appear here.
              </p>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

/**
 * Build a human-readable impact summary from contribution stats.
 * Highlights the most significant content-creation stats (up to 4),
 * then falls back to total_contributions.
 */
function buildImpactSummary(stats: ContributionStats): string {
  const parts: string[] = []

  if (stats.shows_submitted > 0) {
    parts.push(`${stats.shows_submitted} show${stats.shows_submitted !== 1 ? 's' : ''}`)
  }
  if (stats.venues_submitted > 0) {
    parts.push(`${stats.venues_submitted} venue${stats.venues_submitted !== 1 ? 's' : ''}`)
  }
  if (stats.releases_created > 0) {
    parts.push(`${stats.releases_created} release${stats.releases_created !== 1 ? 's' : ''}`)
  }
  if (stats.labels_created > 0) {
    parts.push(`${stats.labels_created} label${stats.labels_created !== 1 ? 's' : ''}`)
  }
  if (stats.festivals_created > 0) {
    parts.push(`${stats.festivals_created} festival${stats.festivals_created !== 1 ? 's' : ''}`)
  }
  if (stats.artists_edited > 0) {
    parts.push(`${stats.artists_edited} artist edit${stats.artists_edited !== 1 ? 's' : ''}`)
  }
  if (stats.revisions_made > 0) {
    parts.push(`${stats.revisions_made} revision${stats.revisions_made !== 1 ? 's' : ''}`)
  }

  if (parts.length === 0) {
    return `You've made ${stats.total_contributions} contribution${stats.total_contributions !== 1 ? 's' : ''} to the knowledge graph.`
  }

  if (parts.length === 1) {
    return `You've contributed ${parts[0]} to the knowledge graph.`
  }

  const last = parts.pop()
  return `You've contributed ${parts.join(', ')} and ${last} to the knowledge graph.`
}
