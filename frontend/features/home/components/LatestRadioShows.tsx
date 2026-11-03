'use client'

import Link from 'next/link'
import { ChevronRight } from 'lucide-react'
import { useStationEpisodes, useStationNowPlaying } from '@/features/radio/hooks'
import { cn } from '@/lib/utils'
import { HOME_ACCENT_LINK_CLASS } from './HomeCityShowsLink'

/**
 * LatestRadioShows (PSY-389, real data per PSY-1329): three station previews,
 * cards on the logged-out homepage (Figma `491:29`, section "Latest radio
 * shows"), rows on the signed-in home (Signed-in Home v2, `2167:33`).
 *
 * The call sign / city / one-line vibe are EDITORIAL (hand-written per
 * station, stable facts + curation — fine to hardcode). The show name and
 * artist line come from the station's newest imported episode
 * (GET /radio-stations/{slug}/episodes, the same feed the /radio hub tables
 * read), and the "live" dot from the PSY-1022 now-playing endpoint's `on_air`
 * — the same truthful signal as the /radio dial strips. PSY-1329 replaced the
 * original PSY-389 placeholder data (fictional shows/artists + an always-on
 * KEXP dot) with these.
 *
 * Degrades gracefully: while loading, or for a station with no imported
 * episodes yet, the card or row renders its editorial shell (call sign / city /
 * vibe) without a show line, never fictional data. Both deep-link to the
 * station's own /radio tab.
 */

interface StationEditorial {
  /** Station call sign (KEXP / WFMU / NTS). */
  station: string
  /** Station slug — the /radio/{slug} tab + API identifier. */
  slug: string
  /** Home city, shown next to the call sign. */
  city: string
  /** One-line "what it's like" for a newcomer. */
  vibe: string
}

// Slugs must track radio_stations.slug (seeded from backend/internal/seeddata/
// radio.go) — a renamed station silently degrades its card to the editorial
// shell (the episodes fetch 404s) while the href dead-links, so verify here on
// any station-slug migration.
const STATIONS: ReadonlyArray<StationEditorial> = [
  {
    station: 'KEXP',
    slug: 'kexp',
    city: 'Seattle',
    vibe: 'Eclectic host’s-choice',
  },
  {
    station: 'WFMU',
    slug: 'wfmu',
    city: 'Jersey City',
    vibe: 'Freeform, anything-goes',
  },
  {
    station: 'NTS',
    slug: 'nts-radio',
    city: 'London',
    // Station-level line (the placeholder's "Global morning show" described
    // its fictional show, not the station) — owner may reword.
    vibe: 'Global, two live channels',
  },
]

/** Max artist names in the card's artist line and in both forms' accessible
 *  name (the row shows no artist line). */
const ARTIST_PREVIEW_CAP = 3

/**
 * How the section draws its stations. `cards` is the logged-out homepage's
 * contract (see AnonymousHome); `rows` is the signed-in home's list, one
 * vocabulary at every width.
 */
export type LatestRadioShowsVariant = 'cards' | 'rows'

/**
 * One station's preview, read once and shared by both forms so the card and
 * the row cannot disagree on what is live or what aired last.
 */
function useStationPreview(editorial: StationEditorial) {
  const { data: episodesData } = useStationEpisodes({
    stationSlug: editorial.slug,
    limit: 1,
  })
  const { data: nowPlaying } = useStationNowPlaying(editorial.slug)

  const latest = episodesData?.episodes?.[0]
  const artists = (latest?.artist_preview ?? [])
    .map(a => a.artist_name)
    .filter(Boolean)
    .slice(0, ARTIST_PREVIEW_CAP)
  const live = nowPlaying?.on_air === true

  // aria-label REPLACES the accessible name computed from the link's content.
  // One composition serves both forms: station · city, live status, latest
  // show, up to ARTIST_PREVIEW_CAP artists. The editorial vibe is not in it,
  // and the row's visible line order (station · show over city) is not the
  // name's order, so the name is not a superset of the visible text.
  const ariaLabel = [
    `${editorial.station} · ${editorial.city}`,
    live ? '— live now' : '',
    latest ? `— latest: ${latest.show_name}` : '',
    artists.length > 0 ? `with ${artists.join(', ')}` : '',
    '. Open the station page.',
  ]
    .filter(Boolean)
    .join(' ')
    .replace(' .', '.')

  return { latest, artists, live, ariaLabel }
}

function StationCard({ editorial }: { editorial: StationEditorial }) {
  const { latest, artists, live, ariaLabel } = useStationPreview(editorial)

  return (
    <Link
      href={`/radio/${editorial.slug}`}
      aria-label={ariaLabel}
      className="flex min-h-[134px] flex-1 flex-col gap-[7px] rounded-xl border border-border bg-card p-[18px] transition-colors hover:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-[7px]">
          <span className="font-data text-sm font-bold text-foreground">
            {editorial.station}
          </span>
          <span className="text-xs text-muted-foreground">{editorial.city}</span>
        </div>
        {live && (
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="text-[8px] text-primary" aria-hidden>
              ●
            </span>
            live
          </span>
        )}
      </div>
      {latest && (
        <p className="text-[15px] font-semibold text-foreground">
          {latest.show_name}
        </p>
      )}
      <p className="text-xs text-muted-foreground">{editorial.vibe}</p>
      {artists.length > 0 && (
        <p className="text-[13px] font-medium text-foreground">
          {artists.join(' · ')}
        </p>
      )}
    </Link>
  )
}

/**
 * The signed-in home's station row (Figma `v2 / Radio row` 2167:79): an 8px
 * live-dot slot, "station · latest show" over "city · live now" or, off air,
 * "city · vibe", and a trailing chevron. The dot slot holds its width when the
 * station is off air so every row's text starts on the same edge.
 */
function StationRow({ editorial }: { editorial: StationEditorial }) {
  const { latest, live, ariaLabel } = useStationPreview(editorial)

  return (
    <Link
      href={`/radio/${editorial.slug}`}
      aria-label={ariaLabel}
      className="group flex min-h-12 items-center gap-2.5 py-[7px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      <span
        aria-hidden
        data-live={live || undefined}
        className={cn('size-2 shrink-0 rounded-full', live && 'bg-primary')}
      />
      <span className="flex min-w-0 flex-1 flex-col gap-px">
        <span className="truncate text-sm leading-[18px] text-foreground">
          {editorial.station}
          {latest && ` · ${latest.show_name}`}
        </span>
        <span className="truncate text-xs leading-[14px] text-muted-foreground">
          {editorial.city} · {live ? 'live now' : editorial.vibe}
        </span>
      </span>
      <ChevronRight
        aria-hidden
        className="size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
      />
    </Link>
  )
}

export function LatestRadioShows({
  variant,
}: {
  variant: LatestRadioShowsVariant
}) {
  const asRows = variant === 'rows'
  return (
    <section aria-labelledby="home-radio-heading" className="flex w-full flex-col gap-4">
      <div className="flex items-center justify-between gap-4">
        <h2
          id="home-radio-heading"
          className="min-w-0 text-2xl font-semibold tracking-tight text-foreground"
        >
          Latest radio shows
        </h2>
        <Link
          href="/radio"
          className={cn(
            'shrink-0 whitespace-nowrap',
            asRows
              ? HOME_ACCENT_LINK_CLASS
              : 'text-sm font-medium text-muted-foreground transition-colors hover:text-link hover:underline underline-offset-4'
          )}
        >
          {asRows ? 'All stations' : 'Browse all stations'} →
        </Link>
      </div>
      {asRows ? (
        <ul className="border-t border-border">
          {STATIONS.map(editorial => (
            <li key={editorial.slug} className="border-b border-border">
              <StationRow editorial={editorial} />
            </li>
          ))}
        </ul>
      ) : (
        <div className="flex flex-col gap-4 sm:flex-row sm:items-stretch">
          {STATIONS.map(editorial => (
            <StationCard key={editorial.slug} editorial={editorial} />
          ))}
        </div>
      )}
    </section>
  )
}
