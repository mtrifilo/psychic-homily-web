'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Lock, CalendarDays, Clock } from 'lucide-react'
import { ActivityHeatmap } from './ActivityHeatmap'
import { UserTierBadge } from './UserTierBadge'
import { ContributionStatsGrid } from './ContributionStatsGrid'
import { ContributionTimeline } from './ContributionTimeline'
import { PercentileRankings } from './PercentileRankings'
import { ProfileSections } from './ProfileSections'
import {
  usePublicProfile,
  usePublicContributions,
} from '@/features/auth'

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
    <div className="container max-w-4xl mx-auto px-4 py-12">
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

export function PublicProfile({ username }: PublicProfileProps) {
  const {
    data: profile,
    isLoading,
    error,
  } = usePublicProfile(username)

  const {
    data: contributionsData,
  } = usePublicContributions(username, { limit: 10 })

  if (isLoading) {
    return <ProfileSkeleton />
  }

  if (error) {
    const apiError = error as { status?: number }
    if (apiError.status === 404) {
      return (
        <div className="container max-w-4xl mx-auto px-4 py-12">
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
      <div className="container max-w-4xl mx-auto px-4 py-12">
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

  // Private profile
  if (profile.profile_visibility === 'private') {
    return (
      <div className="container max-w-4xl mx-auto px-4 py-12">
        <div className="text-center py-16">
          <div className="inline-flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Lock className="h-8 w-8 text-muted-foreground" />
          </div>
          <h1 className="text-2xl font-bold mb-2">Private Profile</h1>
          <p className="text-muted-foreground">
            This user&apos;s profile is set to private.
          </p>
        </div>
      </div>
    )
  }

  const displayName = profile.first_name || profile.username
  const contributions = contributionsData?.contributions || []
  const showStats = profile.stats !== undefined
  const showContributions = contributions.length > 0
  const showSections = profile.sections && profile.sections.length > 0

  return (
    <div className="container max-w-4xl mx-auto px-4 py-12">
      {/* Profile Header */}
      <div className="mb-8">
        <div className="flex items-start gap-4">
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

          <div className="flex-1">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-2xl font-bold">{displayName}</h1>
              <UserTierBadge tier={profile.user_tier} />
            </div>
            <p className="text-sm text-muted-foreground mt-0.5">
              @{profile.username}
            </p>
            {profile.bio && (
              <p className="text-sm mt-2">{profile.bio}</p>
            )}

            {/* Meta info */}
            <div className="flex items-center gap-4 mt-3 text-xs text-muted-foreground">
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
      </div>

      {/* Stats Count Only */}
      {!showStats && profile.stats_count !== undefined && profile.stats_count > 0 && (
        <Card className="mb-6 bg-muted/30 border-border/50">
          <CardContent className="p-4">
            <p className="text-sm text-muted-foreground">
              <span className="font-bold text-foreground text-lg">{profile.stats_count}</span>{' '}
              total contributions
            </p>
          </CardContent>
        </Card>
      )}

      {/* Contribution Stats */}
      {showStats && profile.stats && (
        <section className="mb-8">
          <h2 className="text-lg font-semibold mb-4">Contributions</h2>
          <ContributionStatsGrid stats={profile.stats} />
        </section>
      )}

      {/* Activity Heatmap */}
      {(showStats || (profile.stats_count !== undefined && profile.stats_count > 0)) && (
        <section className="mb-8">
          <h2 className="text-lg font-semibold mb-4">Activity</h2>
          <ActivityHeatmap username={username} />
        </section>
      )}

      {/* Percentile Rankings */}
      <section className="mb-8">
        <PercentileRankings username={username} />
      </section>

      {/* Recent Activity */}
      {showContributions && (
        <section className="mb-8">
          <h2 className="text-lg font-semibold mb-4">Recent Activity</h2>
          <Card className="bg-muted/30 border-border/50">
            <CardContent className="p-2">
              <ContributionTimeline contributions={contributions} />
            </CardContent>
          </Card>
        </section>
      )}

      {/* Custom Sections */}
      {showSections && profile.sections && (
        <section className="mb-8">
          <ProfileSections sections={profile.sections} />
        </section>
      )}

      {/* Empty state when profile is public but has no content */}
      {!showStats && !showContributions && !showSections && profile.stats_count === undefined && (
        <Card className="bg-muted/30 border-border/50">
          <CardContent className="p-8 text-center">
            <p className="text-sm text-muted-foreground">
              This user hasn&apos;t added any content to their profile yet.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
