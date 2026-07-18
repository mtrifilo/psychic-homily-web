'use client'

/**
 * ExplorePage (PSY-837)
 *
 * Legacy /explore landing composition. The route permanently redirects to
 * /graph (PSY-1457); this module remains for Upcoming Shows / Shuffle CTA
 * pieces still exercised by unit tests. Featured Bill/Collection editorial
 * slots were retired in PSY-1480.
 */

import Link from 'next/link'
import type { GeoLocation } from '@/lib/geo-default'
import { UpcomingShowsList } from './UpcomingShowsList'
import { ShuffleCta } from './ShuffleCta'

interface ExplorePageProps {
  /**
   * IP-geo soft default for anon visitors (PSY-926), resolved server-side
   * from the Vercel edge geo headers. Carries the visitor's `{city,state}`
   * plus optional lat/long (PSY-981). A suggestion only — UpcomingShowsList
   * pre-selects the city that has upcoming shows (exact match, else the
   * nearest has-shows city by the visitor's coords) when the visitor is anon
   * with no `?cities=` and no favorites. `null` → "All cities".
   */
  geoDefaultCity?: GeoLocation | null
}

export function ExplorePage({ geoDefaultCity = null }: ExplorePageProps) {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <div className="w-full max-w-6xl px-4 py-8 md:px-8">
        <header className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Explore</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Hand-picked bills, the artists they connect to, and a chronological
            view of everything coming up.
          </p>
        </header>

        <section className="mb-14">
          <div className="flex justify-between items-center mb-5">
            <h2 className="text-2xl font-bold tracking-tight">Upcoming Shows</h2>
            <Link
              href="/shows"
              className="text-sm text-muted-foreground hover:text-primary transition-colors hover:underline underline-offset-4"
            >
              View all shows →
            </Link>
          </div>
          <UpcomingShowsList limit={5} geoDefaultCity={geoDefaultCity} />
        </section>

        <section className="mb-14">
          <div className="mb-3">
            <h2 className="text-2xl font-bold tracking-tight">Surprise me</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Drop into a random artist&apos;s page from the next or last 90
              days of shows.
            </p>
          </div>
          <ShuffleCta />
        </section>
      </div>
    </div>
  )
}
