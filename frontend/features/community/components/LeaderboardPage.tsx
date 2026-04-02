'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Trophy, Medal, Award, Crown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useLeaderboard } from '../hooks/useLeaderboard'
import type { LeaderboardDimension, LeaderboardPeriod, LeaderboardEntry } from '../types'
import { DIMENSION_LABELS, PERIOD_LABELS } from '../types'

const dimensions: LeaderboardDimension[] = ['overall', 'shows', 'venues', 'tags', 'edits', 'requests']
const periods: LeaderboardPeriod[] = ['all_time', 'month', 'week']

function RankIcon({ rank }: { rank: number }) {
  if (rank === 1) return <Trophy className="h-5 w-5 text-yellow-500" />
  if (rank === 2) return <Medal className="h-5 w-5 text-gray-400" />
  if (rank === 3) return <Award className="h-5 w-5 text-amber-600" />
  return <span className="text-sm font-mono text-muted-foreground w-5 text-center">{rank}</span>
}

function TierBadge({ tier }: { tier: string }) {
  const labels: Record<string, string> = {
    new_user: 'New',
    contributor: 'Contributor',
    trusted_contributor: 'Trusted',
    moderator: 'Moderator',
    admin: 'Admin',
  }
  const colors: Record<string, string> = {
    new_user: 'bg-muted text-muted-foreground',
    contributor: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
    trusted_contributor: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
    moderator: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
    admin: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  }
  return (
    <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', colors[tier] || colors.new_user)}>
      {labels[tier] || tier}
    </span>
  )
}

function LeaderboardSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 10 }).map((_, i) => (
        <div key={i} className="flex items-center gap-4 rounded-lg border border-border p-3 animate-pulse">
          <div className="w-5 h-5 rounded-full bg-muted" />
          <div className="w-8 h-8 rounded-full bg-muted" />
          <div className="flex-1 space-y-1">
            <div className="h-4 w-32 rounded bg-muted" />
          </div>
          <div className="h-4 w-16 rounded bg-muted" />
        </div>
      ))}
    </div>
  )
}

function LeaderboardRow({ entry, isCurrentUser }: { entry: LeaderboardEntry; isCurrentUser: boolean }) {
  return (
    <div
      className={cn(
        'flex items-center gap-4 rounded-lg border p-3 transition-colors',
        isCurrentUser
          ? 'border-primary/50 bg-primary/5'
          : 'border-border hover:bg-muted/50'
      )}
    >
      <div className="flex items-center justify-center w-8">
        <RankIcon rank={entry.rank} />
      </div>

      <div className="flex h-8 w-8 items-center justify-center rounded-full bg-muted text-sm font-medium">
        {entry.avatar_url ? (
          <img
            src={entry.avatar_url}
            alt={entry.username}
            className="h-8 w-8 rounded-full object-cover"
          />
        ) : (
          entry.username.charAt(0).toUpperCase()
        )}
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <Link
            href={`/users/${entry.username}`}
            className="text-sm font-medium hover:underline truncate"
          >
            {entry.username}
          </Link>
          <TierBadge tier={entry.user_tier} />
          {isCurrentUser && (
            <span className="text-xs text-primary font-medium">(you)</span>
          )}
        </div>
      </div>

      <div className="text-sm font-semibold tabular-nums">
        {entry.count.toLocaleString()}
      </div>
    </div>
  )
}

export function LeaderboardPage() {
  const [dimension, setDimension] = useState<LeaderboardDimension>('overall')
  const [period, setPeriod] = useState<LeaderboardPeriod>('all_time')
  const { user } = useAuthContext()

  const { data, isLoading, isError } = useLeaderboard(dimension, period)

  return (
    <div className="space-y-6">
      {/* Dimension tabs */}
      <div className="flex flex-wrap gap-1 rounded-lg bg-muted p-1" role="tablist">
        {dimensions.map((dim) => (
          <button
            key={dim}
            role="tab"
            aria-selected={dimension === dim}
            onClick={() => setDimension(dim)}
            className={cn(
              'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
              dimension === dim
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            {DIMENSION_LABELS[dim]}
          </button>
        ))}
      </div>

      {/* Period filter */}
      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">Period:</span>
        <select
          value={period}
          onChange={(e) => setPeriod(e.target.value as LeaderboardPeriod)}
          className="rounded-md border border-input bg-background px-3 py-1.5 text-sm"
        >
          {periods.map((p) => (
            <option key={p} value={p}>
              {PERIOD_LABELS[p]}
            </option>
          ))}
        </select>
      </div>

      {/* Leaderboard content */}
      {isLoading ? (
        <LeaderboardSkeleton />
      ) : isError ? (
        <div className="text-center py-12 text-muted-foreground">
          Failed to load leaderboard. Please try again.
        </div>
      ) : data && data.entries.length === 0 ? (
        <div className="text-center py-12">
          <Crown className="mx-auto h-12 w-12 text-muted-foreground/50 mb-4" />
          <h3 className="text-lg font-medium text-foreground mb-1">No contributions yet</h3>
          <p className="text-sm text-muted-foreground">
            Be the first! Start by{' '}
            <Link href="/contribute" className="text-primary hover:underline">
              contributing
            </Link>{' '}
            to the knowledge graph.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {data?.entries.map((entry) => (
            <LeaderboardRow
              key={entry.user_id}
              entry={entry}
              isCurrentUser={Number(user?.id) === entry.user_id}
            />
          ))}
        </div>
      )}

      {/* Current user's rank */}
      {data?.user_rank && !data.entries.some((e) => e.user_id === Number(user?.id)) && (
        <div className="border-t border-border pt-4">
          <p className="text-sm text-muted-foreground mb-2">Your rank</p>
          <div className="flex items-center gap-4 rounded-lg border border-primary/50 bg-primary/5 p-3">
            <div className="flex items-center justify-center w-8">
              <span className="text-sm font-mono text-muted-foreground w-5 text-center">
                {data.user_rank}
              </span>
            </div>
            <span className="text-sm font-medium">
              You are ranked #{data.user_rank}
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
