'use client'

/**
 * Dense tagged-entity rows (PSY-993 — content-first tag detail rebuild).
 *
 * Replaces the heavyweight per-type entity cards (ArtistCard / ReleaseCard / …)
 * with a single density-first row primitive shared across all entity types:
 *
 *     [ primary name ]  · secondary           right-aligned metric
 *
 * - primary: bold, links to the entity detail page.
 * - secondary: muted, ` · `-joined context (location / artist · type / venue ·
 *   location). Omitted when empty.
 * - metric: right-aligned Space-Mono data point per the design (12 shows / 1991
 *   / Jul 28 / 34 releases). Omitted when not applicable.
 *
 * A generic <TaggedEntityRow> renders the shape; getRowFields maps each
 * TaggedEntityItem's per-type fields onto {primary, secondary, metric, href}.
 * The tag detail page (TagDetail) renders these inside a borderless dense list,
 * mirroring the PSY-992 /tags browse DenseTable treatment.
 */

import Link from 'next/link'
import { BadgeCheck } from 'lucide-react'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { formatInTimezone } from '@/lib/utils/timeUtils'
import type { TaggedEntityItem } from '../types'
import { getEntityUrl } from '../types'

// ──────────────────────────────────────────────
// Per-type field mapping
// ──────────────────────────────────────────────

interface RowFields {
  /** Entity detail URL. */
  href: string
  /** Bold primary label (the entity name). */
  primary: string
  /** Muted, ` · `-joined secondary context. Empty when none applies. */
  secondary: string[]
  /** Right-aligned Space-Mono metric (e.g. "12 shows", "1991"). Null = omit. */
  metric: string | null
  /** True for verified venues — renders a check beside the primary name. */
  verified?: boolean
}

/** Join city/state into "Phoenix, AZ" (either may be missing). */
function locationOf(item: TaggedEntityItem): string | null {
  const parts = [item.city, item.state].filter(Boolean)
  return parts.length > 0 ? parts.join(', ') : null
}

/** Pluralize a count: countLabel(12, "show") -> "12 shows". */
function countLabel(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`
}

/** "Jul 28" in the show's venue timezone (matches the design metric format). */
function formatShowMetric(item: TaggedEntityItem): string | null {
  if (!item.event_date) return null
  const tz = resolveShowTimezone(item.state ?? null, null)
  return formatInTimezone(item.event_date, tz, {
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Map a TaggedEntityItem to its dense-row fields. Keeping the per-type knowledge
 * here lets TagDetail stay agnostic about which backend fields each type
 * populates (mirrors the prior card-adapter split).
 */
export function getRowFields(item: TaggedEntityItem): RowFields {
  const href = getEntityUrl(item.entity_type, item.slug)
  const location = locationOf(item)

  switch (item.entity_type) {
    case 'artist': {
      const upcoming = item.upcoming_show_count ?? 0
      return {
        href,
        primary: item.name,
        secondary: location ? [location] : [],
        metric: upcoming > 0 ? countLabel(upcoming, 'show') : null,
      }
    }
    case 'venue': {
      const upcoming = item.upcoming_show_count ?? 0
      return {
        href,
        primary: item.name,
        secondary: location ? [location] : [],
        metric: countLabel(upcoming, 'show'),
        verified: item.verified ?? false,
      }
    }
    case 'release': {
      // "Loveless · My Bloody Valentine · LP" — but the tag-entities endpoint
      // doesn't return per-release artist credits, so secondary carries the
      // release type only. Metric = release year.
      const secondary: string[] = []
      if (item.release_type) secondary.push(item.release_type.toUpperCase())
      return {
        href,
        primary: item.name,
        secondary,
        metric: item.release_year ? String(item.release_year) : null,
      }
    }
    case 'show': {
      // "Whirr · The Rebel Lounge · Phoenix, AZ" — headliner leads, venue +
      // location in the secondary. Metric = show date.
      const secondary: string[] = []
      const venue = item.venue_name
      if (venue) secondary.push(venue)
      if (location) secondary.push(location)
      return {
        href,
        primary: item.headliner_name || item.name || 'Show',
        secondary,
        metric: formatShowMetric(item),
      }
    }
    case 'label': {
      const releaseCount = item.release_count ?? 0
      return {
        href,
        primary: item.name,
        secondary: location ? [location] : [],
        metric: releaseCount > 0 ? countLabel(releaseCount, 'release') : null,
      }
    }
    case 'festival': {
      const secondary: string[] = []
      if (location) secondary.push(location)
      const artistCount = item.artist_count ?? 0
      return {
        href,
        primary: item.name,
        secondary,
        metric: item.edition_year
          ? String(item.edition_year)
          : artistCount > 0
            ? countLabel(artistCount, 'artist')
            : null,
      }
    }
    case 'collection':
      return {
        href,
        primary: item.name,
        secondary: [],
        metric: null,
      }
    default:
      return {
        href: href === '#' ? getEntityUrl(item.entity_type, item.slug) : href,
        primary: item.name,
        secondary: [],
        metric: null,
      }
  }
}

// ──────────────────────────────────────────────
// Dense row
// ──────────────────────────────────────────────

interface TaggedEntityRowProps {
  item: TaggedEntityItem
}

/**
 * A single dense tagged-entity row. The whole left cluster (name + secondary)
 * sits under one link target; the metric is plain text on the right so the row
 * keeps a single anchor (avoids nested-link strict-mode collisions).
 */
export function TaggedEntityRow({ item }: TaggedEntityRowProps) {
  const { href, primary, secondary, metric, verified } = getRowFields(item)

  return (
    <div
      className="flex items-baseline justify-between gap-3 border-b border-border/30 py-2 last:border-b-0"
      data-testid={`tagged-row-${item.entity_type}`}
    >
      <div className="min-w-0 flex-1 truncate">
        <Link
          href={href}
          className="font-semibold text-foreground hover:underline"
        >
          {primary}
        </Link>
        {verified && (
          <BadgeCheck
            className="ml-1 inline-block h-3.5 w-3.5 -translate-y-px text-primary"
            aria-label="Verified venue"
          />
        )}
        {secondary.length > 0 && (
          <span className="text-sm text-muted-foreground">
            {secondary.map((part, idx) => (
              <span key={idx}>
                <span className="mx-1.5 text-muted-foreground/40" aria-hidden>
                  ·
                </span>
                {part}
              </span>
            ))}
          </span>
        )}
      </div>
      {metric && (
        <span className="shrink-0 font-mono text-sm tabular-nums text-muted-foreground">
          {metric}
        </span>
      )}
    </div>
  )
}
