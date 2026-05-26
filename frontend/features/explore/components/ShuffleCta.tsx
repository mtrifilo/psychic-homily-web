'use client'

/**
 * ShuffleCta (PSY-837)
 *
 * "Drop me somewhere" outline button. Each click pulls a fresh random
 * artist from the ±90-day show pool (`useShuffleTarget` is disabled on
 * mount; we `refetch()` per click so each press is a new pick), then
 * router-pushes to that artist's detail page.
 *
 * When the database has no qualifying artists, the endpoint returns
 * all-null fields with HTTP 200 — we surface a brief inline message
 * rather than navigate. No toast library (per
 * `pattern_mutation_feedback.md`).
 */

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { useShuffleTarget } from '../hooks'

export function ShuffleCta() {
  const router = useRouter()
  const { refetch, isFetching } = useShuffleTarget()
  const [error, setError] = useState<string | null>(null)

  const handleClick = async () => {
    setError(null)
    try {
      const result = await refetch()
      const target = result.data
      if (target?.artist_slug) {
        router.push(`/artists/${target.artist_slug}`)
        return
      }
      setError('No artists available to shuffle to right now.')
    } catch {
      setError('Could not pick a shuffle target — try again in a moment.')
    }
  }

  return (
    <div className="flex flex-col items-start gap-2">
      <button
        type="button"
        onClick={handleClick}
        disabled={isFetching}
        className="inline-flex items-center px-4 py-2 text-sm rounded-lg border border-border/60 hover:bg-muted/50 hover:border-border transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {isFetching ? 'Picking…' : 'Drop me somewhere →'}
      </button>
      {error && (
        <p className="text-sm text-muted-foreground" role="status">
          {error}
        </p>
      )}
    </div>
  )
}
