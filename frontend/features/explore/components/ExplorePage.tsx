'use client'

/**
 * ExplorePage (PSY-837)
 *
 * Client component that composes the /explore landing sections in the
 * order locked in the brief (`docs/open-questions/explore-landing.md`):
 *
 *   1. Page heading
 *   2. Upcoming Shows (chronological)
 *   3. Featured (admin-curated bill) — collapses if null
 *   4. Inline graph — lazy-mounts via IntersectionObserver, anchored
 *      to the featured bill's headliner
 *   5. Featured Collection — collapses if null
 *   6. "Drop me somewhere" shuffle CTA
 *
 * Reads from the SSR-prefetched TanStack Query cache — both Upcoming
 * Shows and Featured are seeded by `app/explore/page.tsx` so the first
 * paint is synchronous, no spinner. The shuffle endpoint is fetched on
 * demand (button click). The graph is fully client-side and only
 * mounts once scrolled into view.
 */

import Link from 'next/link'
import type { GeoLocation } from '@/lib/geo-default'
import { useExploreFeatured } from '../hooks'
import { UpcomingShowsList } from './UpcomingShowsList'
import { FeaturedBillCard } from './FeaturedBillCard'
import { FeaturedCollectionCard } from './FeaturedCollectionCard'
import { InlineGraph } from './InlineGraph'
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
  const { data: featured } = useExploreFeatured()
  const bill = featured?.bill ?? null
  const collection = featured?.collection ?? null

  return (
    <div className="flex min-h-screen items-start justify-center">
      <div className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* 1. Heading */}
        <header className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Explore</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Hand-picked bills, the artists they connect to, and a chronological
            view of everything coming up.
          </p>
        </header>

        {/* 2. Upcoming Shows */}
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

        {/* 3. Featured Bill (collapses when null) */}
        {bill && (
          <section className="mb-14">
            <div className="flex justify-between items-center mb-5">
              <h2 className="text-2xl font-bold tracking-tight">Featured</h2>
            </div>
            <FeaturedBillCard bill={bill} />
          </section>
        )}

        {/* 4. Inline knowledge graph anchored to the featured bill */}
        {bill && (
          <section className="mb-14">
            <div className="flex justify-between items-center mb-5">
              <h2 className="text-2xl font-bold tracking-tight">
                Explore the lineup
              </h2>
            </div>
            <InlineGraph
              billSlug={bill.slug}
              billTitle={bill.title}
              billHref={`/shows/${bill.slug || bill.id}`}
            />
          </section>
        )}

        {/* 5. Featured Collection (collapses when null) */}
        {collection && (
          <section className="mb-14">
            <div className="flex justify-between items-center mb-5">
              <h2 className="text-2xl font-bold tracking-tight">
                Featured Collection
              </h2>
              <Link
                href="/collections"
                className="text-sm text-muted-foreground hover:text-primary transition-colors hover:underline underline-offset-4"
              >
                View all collections →
              </Link>
            </div>
            <FeaturedCollectionCard collection={collection} />
          </section>
        )}

        {/* 6. Shuffle CTA */}
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
