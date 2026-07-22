'use client'

import Link from 'next/link'
import { Library } from 'lucide-react'
import { UserAttribution } from '@/components/shared'
import { CollectionCoverImage } from '@/features/collections/components/CollectionCoverImage'
import { useFeaturedCollection, useFeaturedCollectionHistory } from '../hooks'
import type { FeaturedCollectionRun } from '../types'

const linkClass =
  'hover:text-primary focus-visible:text-primary focus-visible:outline-none'

/**
 * Full-date label for the run's start. Estimated starts (reconstructed at
 * backfill, PSY-1500) render as approximate — "featured before <date>" — so
 * the card never presents a fabricated timestamp as a precise fact.
 */
function featuredDateLabel(run: FeaturedCollectionRun): string {
  const date = new Date(run.featured_at)
  if (Number.isNaN(date.getTime())) return 'featured recently'
  const formatted = date.toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
    timeZone: 'UTC',
  })
  return run.featured_at_estimated
    ? `featured before ${formatted}`
    : `featured ${formatted}`
}

function countLabel(count: number, singular: string): string {
  return `${count} ${count === 1 ? singular : `${singular}s`}`
}

/**
 * Broadsheet "Featured Collection" editorial slot (PSY-1411). Sits at the foot
 * of the live `/charts` Broadsheet, below the module grid, per approved Figma
 * board A. Self-fetches the live pick (the open feature run with the newest
 * `featured_at`) and self-hides when nothing is featured — the charts zero-row
 * rule: no placeholder, no empty card.
 *
 * Treatment follows the Almanac editorial card: 104px cover, `FEATURED
 * COLLECTION` mono label, title, a blurb from the collection's own description,
 * and a muted meta line. The meta row carries a `discuss →` link into the
 * collection's comment thread (`#discussion`), and a `previously featured →`
 * link to the archive renders below the card ONLY when a closed run exists —
 * gated like PSY-1422's "Archives:" line.
 */
export function FeaturedCollectionCard() {
  const { data, isLoading, isError } = useFeaturedCollection()
  const run = data?.featured ?? null

  // History is only consulted to decide whether the archive link is warranted,
  // so it's skipped entirely until a pick actually renders (no extra request on
  // the far-more-common nothing-featured path).
  const history = useFeaturedCollectionHistory(20, 0, {
    enabled: Boolean(run),
  })
  // The archive is worth linking only once a genuine closed run exists — a set
  // of currently-open runs are all "featured", none "previously". Newest-first,
  // so a closed run surfaces on the first page in every realistic case (manual,
  // rare unfeaturing per lock 2A keeps concurrent open runs few).
  const hasClosedRuns = Boolean(
    history.data?.runs?.some((r) => r.unfeatured_at !== null)
  )

  // Zero-row rule: nothing (loading, error, or genuinely unfeatured) renders
  // no card and no placeholder.
  if (isLoading || isError || !run) return null

  const collectionHref = `/collections/${run.slug}`

  return (
    <section
      aria-labelledby="featured-collection-heading"
      data-testid="featured-collection"
      className="border-t-2 border-foreground pt-4"
    >
      <div className="flex gap-4 rounded-md border border-border p-4">
        <CollectionCoverImage
          url={run.cover_image_url}
          alt={`${run.title} cover`}
          className="h-[104px] w-[104px] shrink-0 rounded-md border border-border bg-muted/50"
          fallback={<Library className="h-10 w-10 text-muted-foreground/40" />}
        />
        <div className="flex min-w-0 flex-1 flex-col">
          <p
            id="featured-collection-heading"
            className="font-mono text-[11px] font-bold uppercase tracking-[0.06em] text-primary"
          >
            Featured Collection
          </p>
          <h2 className="mt-1 text-base font-semibold leading-snug">
            <Link href={collectionHref} className={linkClass}>
              {run.title}
            </Link>
          </h2>
          {run.description ? (
            <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">
              {run.description}
            </p>
          ) : null}
          <div className="mt-2 flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <p className="min-w-0 flex-1 text-xs text-muted-foreground">
              curated by{' '}
              <UserAttribution
                name={run.creator_name}
                username={run.creator_username}
                className={linkClass}
              />{' '}
              · {countLabel(run.item_count, 'item')} ·{' '}
              {countLabel(run.subscriber_count, 'subscriber')} ·{' '}
              {featuredDateLabel(run)}
            </p>
            <Link
              href={`${collectionHref}#discussion`}
              className="shrink-0 text-xs text-primary hover:underline focus-visible:underline focus-visible:outline-none"
            >
              discuss →
            </Link>
          </div>
        </div>
      </div>
      {hasClosedRuns ? (
        <p className="mt-2 font-mono text-[11px] text-muted-foreground">
          <Link href="/charts/featured" className="text-primary hover:underline focus-visible:underline focus-visible:outline-none">
            previously featured →
          </Link>
        </p>
      ) : null}
    </section>
  )
}
