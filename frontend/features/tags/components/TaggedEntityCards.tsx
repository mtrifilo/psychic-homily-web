'use client'

/**
 * Tagged-entity card row renderers (PSY-485).
 *
 * Adapts the lightweight `TaggedEntityItem` returned by GET /tags/{slug}/entities
 * into the entity-specific shapes consumed by the existing feature-module cards
 * (ArtistCard, FestivalCard, LabelCard, ReleaseCard) wherever those work with
 * minimal data, and renders venue/show rows inline because the heavyweight
 * VenueCard/ShowCard pull in feature-specific dependencies (favorite buttons,
 * attendance, music embeds) that don't make sense in this context.
 *
 * Each renderer accepts the raw `TaggedEntityItem` so the parent (TagDetail) can
 * stay agnostic about which fields the backend populated for which type.
 */

import Link from 'next/link'
import { BadgeCheck, Calendar, Library, MapPin, Music, Disc3, Tag } from 'lucide-react'
import { ArtistCard } from '@/features/artists'
import { FestivalCard } from '@/features/festivals'
import { LabelCard } from '@/features/labels'
import { ReleaseCard } from '@/features/releases'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import type { TaggedEntityItem } from '../types'
import { getEntityUrl } from '../types'

// ──────────────────────────────────────────────
// Per-type adapters
// ──────────────────────────────────────────────

function TaggedArtistCard({ item }: { item: TaggedEntityItem }) {
  // Adapt to ArtistListItem. ArtistCard only reads name, slug, city, state,
  // and upcoming_show_count, so we can pass a minimal shape with the rest
  // stubbed out.
  const artist = {
    id: item.entity_id,
    slug: item.slug,
    name: item.name,
    city: item.city ?? null,
    state: item.state ?? null,
    bandcamp_embed_url: null,
    description: null,
    social: {
      instagram: null,
      facebook: null,
      twitter: null,
      youtube: null,
      spotify: null,
      soundcloud: null,
      bandcamp: null,
      website: null,
    },
    created_at: '',
    updated_at: '',
    upcoming_show_count: item.upcoming_show_count ?? 0,
  }
  return <ArtistCard artist={artist} density="comfortable" />
}

function TaggedFestivalCard({ item }: { item: TaggedEntityItem }) {
  // Adapt to FestivalListItem. The card reads name, slug, city, state,
  // edition_year, start_date, end_date, status, artist_count, venue_count.
  const festival = {
    id: item.entity_id,
    name: item.name,
    slug: item.slug,
    series_slug: '', // unused by FestivalCard
    edition_year: item.edition_year ?? 0,
    city: item.city || null,
    state: item.state || null,
    start_date: item.start_date ?? '',
    end_date: item.end_date ?? '',
    status: item.status ?? 'announced',
    artist_count: item.artist_count ?? 0,
    venue_count: item.venue_count ?? 0,
  }
  return <FestivalCard festival={festival} density="comfortable" />
}

function TaggedLabelCard({ item }: { item: TaggedEntityItem }) {
  const label = {
    id: item.entity_id,
    name: item.name,
    slug: item.slug,
    city: item.city || null,
    state: item.state || null,
    status: item.status ?? 'active',
    artist_count: item.artist_count ?? 0,
    release_count: item.release_count ?? 0,
  }
  return <LabelCard label={label} density="comfortable" />
}

function TaggedReleaseCard({ item }: { item: TaggedEntityItem }) {
  // ReleaseCard expects ReleaseListItem with an `artists` array. The tag
  // detail endpoint doesn't return per-release artist credits (would require
  // an extra JOIN per row), so we render an empty artists array — the card
  // gracefully handles that case (no artist line when the array is empty).
  const release = {
    id: item.entity_id,
    title: item.name,
    slug: item.slug,
    release_type: item.release_type ?? 'lp',
    release_year: item.release_year ?? null,
    cover_art_url: item.cover_art_url ?? null,
    artist_count: 0,
    artists: [],
    label_name: null,
    label_slug: null,
  }
  return <ReleaseCard release={release} density="comfortable" />
}

/**
 * Inline venue card. We don't reuse VenueCard because it pulls in
 * useVenueShows, FavoriteVenueButton, edit/delete dialogs, and other
 * authenticated affordances that aren't appropriate in this read-only,
 * cross-entity browsing context. The visual treatment matches the artist
 * card so the tabs feel cohesive.
 */
function TaggedVenueRow({ item }: { item: TaggedEntityItem }) {
  const hasLocation = item.city || item.state
  const location = hasLocation
    ? [item.city, item.state].filter(Boolean).join(', ')
    : null
  const upcoming = item.upcoming_show_count ?? 0
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <Link
            href={getEntityUrl('venue', item.slug)}
            className="block group"
          >
            <h3 className="font-bold text-base text-foreground group-hover:text-primary transition-colors truncate inline-flex items-center gap-1.5">
              {item.name}
              {item.verified && (
                <BadgeCheck className="h-4 w-4 text-primary shrink-0" aria-label="Verified venue" />
              )}
            </h3>
          </Link>
          {location && (
            <div className="mt-1 flex items-center gap-1.5 text-sm text-muted-foreground">
              <MapPin className="h-3.5 w-3.5 shrink-0" />
              <span>{location}</span>
            </div>
          )}
        </div>
        <span
          className={`text-xs font-medium px-2 py-1 rounded-full shrink-0 ${
            upcoming > 0
              ? 'bg-primary/10 text-primary'
              : 'bg-muted text-muted-foreground'
          }`}
        >
          {upcoming} {upcoming === 1 ? 'show' : 'shows'}
        </span>
      </div>
    </article>
  )
}

/**
 * Inline show row. ShowCard wants a full ShowResponse with bill positions,
 * attendance counters, edit/delete affordances, and music embed plumbing —
 * none of which the tag-entities endpoint exposes. We render the headliner +
 * venue + date with the same visual rhythm.
 */
function TaggedShowRow({ item }: { item: TaggedEntityItem }) {
  const dateBadge = item.event_date
    ? formatShowDateBadge(item.event_date, item.state ?? null)
    : null
  const headliner = item.headliner_name
  const venue = item.venue_name
  const location =
    item.city && item.state
      ? `${item.city}, ${item.state}`
      : item.city || item.state || null

  return (
    <article className="rounded-lg border border-border/50 bg-card p-3 sm:p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3 sm:gap-4">
        {dateBadge && (
          <Link
            href={getEntityUrl('show', item.slug)}
            className="shrink-0 flex flex-col items-center justify-center rounded-md bg-muted/50 hover:bg-muted transition-colors w-14 sm:w-16 py-2"
          >
            <span className="text-[10px] sm:text-xs font-bold tracking-widest uppercase text-muted-foreground leading-none">
              {dateBadge.dayOfWeek}
            </span>
            <span className="text-xs sm:text-sm font-semibold text-primary leading-tight mt-0.5">
              {dateBadge.monthDay}
            </span>
          </Link>
        )}
        <div className="flex-1 min-w-0">
          <Link href={getEntityUrl('show', item.slug)} className="block group">
            <h3 className="font-bold text-base sm:text-lg text-foreground group-hover:text-primary transition-colors truncate">
              {headliner || item.name || 'Show'}
            </h3>
          </Link>
          {venue && (
            <div className="text-sm text-muted-foreground mt-0.5">
              {item.venue_slug ? (
                <Link
                  href={`/venues/${item.venue_slug}`}
                  className="text-primary/80 hover:text-primary font-medium transition-colors"
                >
                  {venue}
                </Link>
              ) : (
                <span className="text-primary/80 font-medium">{venue}</span>
              )}
              {location && (
                <span className="text-muted-foreground/80">
                  {' '}&middot; {location}
                </span>
              )}
            </div>
          )}
        </div>
      </div>
    </article>
  )
}

/**
 * PSY-553: tagged collection row. The backend (`enrichCollections`) only
 * populates name + slug for collections (no city/state/counts), and the
 * full CollectionCard requires a heavyweight shape with item counts,
 * contributor counts, like state, etc. that the tag-entities endpoint
 * doesn't return. Following the TaggedShowRow / TaggedVenueRow precedent,
 * we render a minimal inline row that links to /collections/{slug}.
 */
function TaggedCollectionRow({ item }: { item: TaggedEntityItem }) {
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-muted/50">
          <Library className="h-5 w-5 text-muted-foreground/70" />
        </div>
        <div className="min-w-0 flex-1">
          <Link
            href={getEntityUrl('collection', item.slug)}
            className="block group"
          >
            <h3 className="font-bold text-base text-foreground group-hover:text-primary transition-colors truncate">
              {item.name}
            </h3>
          </Link>
        </div>
      </div>
    </article>
  )
}

// ──────────────────────────────────────────────
// Public renderer
// ──────────────────────────────────────────────

interface TaggedEntityCardProps {
  item: TaggedEntityItem
}

/**
 * Render a single tagged-entity card based on its type. Any unknown entity
 * type falls back to a minimal link row so we never silently drop data when
 * the backend learns about a new tag-eligible entity type.
 */
export function TaggedEntityCard({ item }: TaggedEntityCardProps) {
  switch (item.entity_type) {
    case 'artist':
      return <TaggedArtistCard item={item} />
    case 'venue':
      return <TaggedVenueRow item={item} />
    case 'festival':
      return <TaggedFestivalCard item={item} />
    case 'label':
      return <TaggedLabelCard item={item} />
    case 'release':
      return <TaggedReleaseCard item={item} />
    case 'show':
      return <TaggedShowRow item={item} />
    case 'collection':
      return <TaggedCollectionRow item={item} />
    default:
      return (
        <article className="rounded-lg border border-border/50 bg-card p-4">
          <Link
            href={getEntityUrl(item.entity_type, item.slug)}
            className="font-medium text-foreground hover:text-primary transition-colors"
          >
            {item.name}
          </Link>
        </article>
      )
  }
}

// Icon helper used by the tab labels — exported alongside so the parent
// component can stay free of per-type icon mapping repetition.
export const ENTITY_TYPE_TAB_ICON: Record<
  string,
  React.ComponentType<{ className?: string }>
> = {
  artist: Music,
  venue: MapPin,
  festival: Calendar,
  label: Tag,
  release: Disc3,
  show: Calendar,
  collection: Library,
}

