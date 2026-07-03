'use client'

import Link from 'next/link'
import { useStationEpisodes, useStationNowPlaying } from '@/features/radio/hooks'

/**
 * LatestRadioShows (PSY-389, real data per PSY-1329) — three station preview
 * cards on the logged-out homepage (Figma `491:29`, section "Latest radio
 * shows").
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
 * episodes yet, the card renders its editorial shell (call sign / city /
 * vibe) without a show line — never fictional data. Cards deep-link to the
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

/** Max artist names shown on the card's one-line artist preview. */
const CARD_ARTIST_CAP = 3

function StationCard({ editorial }: { editorial: StationEditorial }) {
  const { data: episodesData } = useStationEpisodes({
    stationSlug: editorial.slug,
    limit: 1,
  })
  const { data: nowPlaying } = useStationNowPlaying(editorial.slug)

  const latest = episodesData?.episodes?.[0]
  const artists = (latest?.artist_preview ?? [])
    .map(a => a.artist_name)
    .filter(Boolean)
    .slice(0, CARD_ARTIST_CAP)
  const live = nowPlaying?.on_air === true

  // aria-label REPLACES the accessible name computed from the card's content,
  // so everything a sighted user sees must be restated here — notably the
  // live status and artists (the dot glyph is aria-hidden decoration).
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

  return (
    <Link
      href={`/radio/${editorial.slug}`}
      aria-label={ariaLabel}
      className="flex min-h-[134px] flex-1 flex-col gap-[7px] rounded-xl border border-border bg-card p-[18px] transition-colors hover:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-[7px]">
          <span className="font-mono text-sm font-bold text-foreground">
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

export function LatestRadioShows() {
  return (
    <section aria-labelledby="home-radio-heading" className="flex w-full flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2
          id="home-radio-heading"
          className="text-2xl font-semibold tracking-tight text-foreground"
        >
          Latest radio shows
        </h2>
        <Link
          href="/radio"
          className="text-sm font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
        >
          Browse all stations →
        </Link>
      </div>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-stretch">
        {STATIONS.map(editorial => (
          <StationCard key={editorial.slug} editorial={editorial} />
        ))}
      </div>
    </section>
  )
}
