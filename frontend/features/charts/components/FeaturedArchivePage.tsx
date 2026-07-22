'use client'

/**
 * FeaturedArchivePage (PSY-1501)
 *
 * The permalinkable `/charts/featured` archive: a lead editorial card for the
 * most-recent pick followed by dense, reverse-chronological ledger rows for the
 * closed runs. Layout + copy locked by PSY-1493 board B (light node 1076:15,
 * dark node 1077:46).
 *
 * Single source of truth is GET /charts/featured-collection/history, which
 * returns every stint newest-first (open + closed). The newest run peels off as
 * the lead card; the remainder become ledger rows. A run carrying
 * `featured_at_estimated` has no trustworthy start — the migration reconstructed
 * it — so it renders "before <date>" in muted, never a fabricated precise date.
 *
 * The route stays valid (200, never 404) with no picks: it is early, not broken.
 */

import Link from 'next/link'
import { Library } from 'lucide-react'
import { cn } from '@/lib/utils'
import { UserAttribution } from '@/components/shared'
import { CollectionCoverImage } from '@/features/collections/components/CollectionCoverImage'
import { useFeaturedCollectionHistory } from '../hooks'
import type { FeaturedCollectionRun } from '../types'

const linkClass =
  'hover:text-primary focus-visible:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm'

const discussLinkClass =
  'shrink-0 whitespace-nowrap font-mono text-[11px] font-bold text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm'

/** The comment thread lives at the collection detail's #discussion anchor. */
function discussHref(slug: string): string {
  return `/collections/${slug}#discussion`
}

function collectionHref(slug: string): string {
  return `/collections/${slug}`
}

function formatDay(value: string, withYear: boolean): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: '2-digit',
    ...(withYear ? { year: 'numeric' } : {}),
    timeZone: 'UTC',
  })
}

/**
 * The ledger date column. Estimated starts collapse to "before <end>" (muted);
 * real ranges read "MMM DD – MMM DD, YYYY" (design drops the start year in the
 * common same-year case, restores it only when the run spans a year boundary).
 */
function runDateRange(run: FeaturedCollectionRun): {
  text: string
  estimated: boolean
} {
  if (run.featured_at_estimated) {
    // Start is unknown; anchor on the end (or the estimated start itself while
    // the estimated run is somehow still open) so we never invent a precise day.
    const anchor = run.unfeatured_at ?? run.featured_at
    return { text: `before ${formatDay(anchor, true)}`, estimated: true }
  }
  if (!run.unfeatured_at) {
    return { text: `${formatDay(run.featured_at, true)} – present`, estimated: false }
  }
  const start = new Date(run.featured_at)
  const end = new Date(run.unfeatured_at)
  const sameYear =
    !Number.isNaN(start.getTime()) &&
    !Number.isNaN(end.getTime()) &&
    start.getUTCFullYear() === end.getUTCFullYear()
  return {
    text: `${formatDay(run.featured_at, !sameYear)} – ${formatDay(run.unfeatured_at, true)}`,
    estimated: false,
  }
}

/** Lead-card meta tail: "featured <start> — still live" or a closed range. */
function leadFeaturedLabel(run: FeaturedCollectionRun): string {
  const start = run.featured_at_estimated
    ? `before ${formatDay(run.featured_at, true)}`
    : formatDay(run.featured_at, true)
  if (!run.unfeatured_at) return `featured ${start} — still live`
  return `featured ${runDateRange(run).text}`
}

function itemLabel(count: number): string {
  return `${count} ${count === 1 ? 'item' : 'items'}`
}

function CoverFallback() {
  return <Library className="h-8 w-8 text-muted-foreground/40" aria-hidden="true" />
}

function LeadCard({ run }: { run: FeaturedCollectionRun }) {
  return (
    <section className="space-y-2.5">
      <p className="font-mono text-[11px] font-bold tracking-[0.06em] text-primary">
        FEATURED COLLECTION · MOST RECENT
      </p>
      <div className="flex gap-[18px] rounded border border-border bg-card p-[18px]">
        <Link
          href={collectionHref(run.slug)}
          className="shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-[2px]"
          aria-label={run.title}
        >
          <CollectionCoverImage
            url={run.cover_image_url}
            alt={`${run.title} cover`}
            className="size-[104px] rounded-[2px] border border-border bg-background"
            fallback={<CoverFallback />}
          />
        </Link>
        <div className="flex min-w-0 flex-1 flex-col gap-[7px]">
          <h2 className="text-base font-semibold leading-tight text-foreground">
            <Link href={collectionHref(run.slug)} className={linkClass}>
              {run.title}
            </Link>
          </h2>
          {run.description ? (
            <p className="line-clamp-3 text-[13px] leading-[19px] text-foreground">
              {run.description}
            </p>
          ) : null}
          <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1">
            <p className="min-w-0 flex-1 text-[11px] text-muted-foreground">
              curated by{' '}
              <UserAttribution
                name={run.creator_name}
                username={run.creator_username}
                className="text-foreground hover:underline"
              />{' '}
              · {itemLabel(run.item_count)} · {leadFeaturedLabel(run)}
            </p>
            <Link href={discussHref(run.slug)} className={discussLinkClass}>
              discuss →
            </Link>
          </div>
        </div>
      </div>
    </section>
  )
}

function LedgerRow({ run }: { run: FeaturedCollectionRun }) {
  const range = runDateRange(run)
  return (
    <div className="flex items-center gap-4 border-b border-border py-[11px] sm:gap-5">
      <p
        className={cn(
          'w-[120px] shrink-0 font-mono text-[11px] sm:w-[190px]',
          range.estimated ? 'text-muted-foreground' : 'text-foreground'
        )}
      >
        {range.text}
      </p>
      <div className="flex min-w-0 flex-1 flex-col gap-[3px]">
        <p className="truncate text-sm font-semibold text-foreground">
          <Link href={collectionHref(run.slug)} className={linkClass}>
            {run.title}
          </Link>
        </p>
        <p className="truncate text-[11px] text-muted-foreground">
          <UserAttribution
            name={run.creator_name}
            username={run.creator_username}
            className="hover:underline"
          />{' '}
          · {itemLabel(run.item_count)}
          {run.featured_at_estimated
            ? ' · start date reconstructed at migration'
            : ''}
        </p>
      </div>
      <Link href={discussHref(run.slug)} className={discussLinkClass}>
        discuss →
      </Link>
    </div>
  )
}

function PageHeader() {
  return (
    <header className="space-y-2">
      <Link
        href="/charts"
        className="font-mono text-xs text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-sm"
      >
        ← Charts
      </Link>
      <h1 className="font-display text-3xl font-bold leading-none">
        Previously Featured
      </h1>
      <p className="text-[13px] text-muted-foreground">
        Every collection that has held the slot.
      </p>
    </header>
  )
}

export function FeaturedArchivePage() {
  const history = useFeaturedCollectionHistory()

  const runs = history.data?.runs ?? []
  const lead = runs[0]
  const ledger = runs.slice(1)

  return (
    <div className="space-y-[26px]">
      <PageHeader />

      {history.isLoading ? (
        <div className="space-y-4" aria-hidden="true">
          <div className="h-[140px] w-full animate-pulse rounded bg-muted" />
          <div className="space-y-2">
            {Array.from({ length: 4 }, (_, index) => (
              <div
                key={index}
                className="h-10 w-full animate-pulse rounded-sm bg-muted"
              />
            ))}
          </div>
        </div>
      ) : history.isError ? (
        <p className="border-y border-border py-4 text-[13px] text-destructive">
          Unable to load the featured-collection archive.
        </p>
      ) : !lead ? (
        // Charts empty-state convention: one factual line, no illustration. The
        // route stays valid — it is early, not broken.
        <div className="border-y border-border py-4">
          <p className="text-[13px] text-muted-foreground">
            No collection has been featured yet.
          </p>
        </div>
      ) : (
        <>
          <LeadCard run={lead} />

          {ledger.length > 0 ? (
            <section>
              <div className="flex items-center gap-2 border-b-2 border-foreground pb-2">
                <p className="font-mono text-[11px] font-bold tracking-[0.06em] text-foreground">
                  PREVIOUSLY FEATURED
                </p>
                <span className="h-px flex-1" />
                <p className="font-mono text-[11px] text-muted-foreground">
                  {ledger.length}{' '}
                  {ledger.length === 1 ? 'closed run' : 'closed runs'}
                </p>
              </div>
              {ledger.map((run) => (
                <LedgerRow key={run.run_id} run={run} />
              ))}
            </section>
          ) : null}
        </>
      )}
    </div>
  )
}
