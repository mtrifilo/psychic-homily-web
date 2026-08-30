'use client'

import Link from 'next/link'
import { BracketLink, SectionHeader, StatsList } from '@/components/shared'
import type { StatsListItem } from '@/components/shared'
import { useStationEpisodes } from '../hooks/useStationEpisodes'
import { useStationTopArtists } from '../hooks/useStationTopArtists'
import { useStationTopLabels } from '../hooks/useStationTopLabels'
import { useNewReleaseRadar } from '../hooks/useNewReleaseRadar'
import { formatStationLocation } from '../lib/stationOverview'
import { getBroadcastTypeLabel } from '../types'
import type { RadioStationDetail } from '../types'

const TOP_ARTISTS_LIMIT = 8
const TOP_LABELS_LIMIT = 5
const RADAR_LIMIT = 5

interface StationSidebarProps {
  station: RadioStationDetail
}

/**
 * Station-page sidebar (PSY-1050): STATION info box, 90-day top artists /
 * labels (PSY-1048 station-scoped endpoints), and the station-filtered New
 * Release Radar.
 *
 * Note: the mock's "artists matched %" stat needs per-station data the API
 * doesn't expose — dropped for v1 (per ticket).
 */
export function StationSidebar({ station }: StationSidebarProps) {
  return (
    <aside className="flex flex-col gap-6" aria-label="Station details">
      <StationInfoBox station={station} />
      <TopArtistsBox stationSlug={station.slug} />
      <TopLabelsBox stationSlug={station.slug} />
      <NewReleaseRadarBox station={station} />
    </aside>
  )
}

// ---------------------------------------------------------------------------
// STATION info box
// ---------------------------------------------------------------------------

function StationInfoBox({ station }: { station: RadioStationDetail }) {
  // Cheap total-only probe; shares the endpoint (not the cache entry) with
  // the playlists feed. One tiny request buys the "playlists tracked" stat
  // without coupling the sidebar to the feed's pagination state.
  const { data: episodesData } = useStationEpisodes({
    stationSlug: station.slug,
    limit: 1,
  })

  const location = formatStationLocation(station.city, station.state)
  const streams = station.stream_urls ? Object.keys(station.stream_urls) : []

  const items: StatsListItem[] = []
  if (station.frequency_mhz) {
    items.push({ label: 'Frequency', value: `${station.frequency_mhz} MHz` })
  }
  if (location) items.push({ label: 'Broadcasting from', value: location })
  items.push({
    label: 'Broadcast',
    value: getBroadcastTypeLabel(station.broadcast_type),
  })
  if (streams.length > 0) {
    items.push({
      label: 'Streams',
      value: streams.length === 1 ? streams[0] : `${streams.length} streams`,
    })
  }
  if (station.show_count > 0) {
    items.push({ label: 'Shows tracked', value: station.show_count })
  }
  if (episodesData && episodesData.total > 0) {
    items.push({ label: 'Playlists tracked', value: episodesData.total })
  }
  items.push({ label: 'On the graph since', value: formatMonthYear(station.created_at) })

  // ONE policy for every URL in this box: keep only absolute http(s), drop the
  // rest. These columns are operator-entered free text, and `social` is
  // free-form JSONB whose values are sometimes bare handles (SocialLinks builds
  // hrefs from baseUrl + handle). Dropping an unusable value is better than
  // both alternatives: a broken/relative anchor, or a permanently greyed
  // bracket (BracketLink's `external` floor renders one) that looks like a
  // disabled feature rather than bad data.
  //
  // `ariaLabel` names the station on every entry: the labels here are hosts and
  // network keys ("wfmu.org", "bluesky") that say nothing on their own, and the
  // ↗ is a VISUAL marker that should not be read aloud as "north east arrow".
  const links: Array<{ label: string; href: string; ariaLabel: string }> = []
  // Trimmed before the scheme test, matching BracketLink: the write path
  // persists raw operator input, so a pasted "  https://..." must not be
  // mistaken for an unusable value and dropped.
  //
  // Named parameters, not positional: `label` and `ariaLabel` are both strings
  // and both plausible in either slot, so a transposition would typecheck and
  // render a bracket whose visible text and spoken name are swapped.
  const addLink = (link: {
    label: string
    url: string | null | undefined
    ariaLabel: string
  }) => {
    // `typeof` guard, not just a null check: `social` is free-form JSONB with
    // no server-side schema, so its values are only string-typed by assertion.
    // A stored number would make `.trim()` throw during render and take the
    // whole sidebar to the error boundary, where the old code merely skipped
    // the entry.
    const href = typeof link.url === 'string' ? link.url.trim() : ''
    if (href && /^https?:\/\//i.test(href)) {
      links.push({ label: link.label, href, ariaLabel: link.ariaLabel })
    }
  }

  addLink({
    label: 'donate',
    url: station.donation_url,
    ariaLabel: `Donate to ${station.name}`,
  })
  if (station.website) {
    addLink({
      label: `${hostLabel(station.website)} ↗`,
      url: station.website,
      ariaLabel: `${station.name} website`,
    })
  }
  for (const [key, url] of Object.entries(station.social ?? {})) {
    addLink({ label: `${key} ↗`, url, ariaLabel: `${station.name} on ${key}` })
  }

  return (
    <section aria-label="Station">
      <SectionHeader title="Station" />
      <StatsList items={items} />
      {links.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1">
          {links.map(link => (
            <BracketLink
              key={link.label}
              {...link}
              external
              className="font-mono text-xs"
            />
          ))}
        </div>
      )}
    </section>
  )
}

/** "1 play" / "3 plays". */
function pluralize(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`
}

/** "Feb 2026" from an ISO timestamp; empty string when unparseable. */
function formatMonthYear(iso: string): string {
  const date = new Date(iso)
  if (isNaN(date.getTime())) return ''
  return date.toLocaleDateString('en-US', { month: 'short', year: 'numeric' })
}

/** Short display host for a website URL ("wfmu.org"). */
function hostLabel(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return 'website'
  }
}

// ---------------------------------------------------------------------------
// Top artists / labels (90d, station-scoped — PSY-1048)
// ---------------------------------------------------------------------------

function TopArtistsBox({ stationSlug }: { stationSlug: string }) {
  const { data } = useStationTopArtists({ stationSlug, limit: TOP_ARTISTS_LIMIT })
  const artists = data?.artists ?? []
  if (artists.length === 0) return null

  return (
    <section aria-label="Top artists">
      <SectionHeader title="Top artists — last 90 days" />
      <ul className="space-y-0.5 text-sm">
        {artists.map(artist => (
          <li
            key={artist.artist_name}
            className="flex items-baseline justify-between gap-2"
          >
            {artist.artist_slug ? (
              <Link
                href={`/artists/${artist.artist_slug}`}
                className="truncate font-medium text-foreground hover:text-primary transition-colors"
              >
                {artist.artist_name}
              </Link>
            ) : (
              <span className="truncate">{artist.artist_name}</span>
            )}
            <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
              {pluralize(artist.play_count, 'play')} · {artist.episode_count} eps
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function TopLabelsBox({ stationSlug }: { stationSlug: string }) {
  const { data } = useStationTopLabels({ stationSlug, limit: TOP_LABELS_LIMIT })
  const labels = data?.labels ?? []
  if (labels.length === 0) return null

  return (
    <section aria-label="Top labels">
      <SectionHeader title="Top labels — last 90 days" />
      <ul className="space-y-0.5 text-sm">
        {labels.map(label => (
          <li
            key={label.label_name}
            className="flex items-baseline justify-between gap-2"
          >
            {label.label_slug ? (
              <Link
                href={`/labels/${label.label_slug}`}
                className="truncate font-medium text-foreground hover:text-primary transition-colors"
              >
                {label.label_name}
              </Link>
            ) : (
              <span className="truncate">{label.label_name}</span>
            )}
            <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
              {pluralize(label.play_count, 'play')}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}

// ---------------------------------------------------------------------------
// New Release Radar (station-filtered; single-station semantics, consistent
// with every other station-scoped read since PSY-1074)
// ---------------------------------------------------------------------------

function NewReleaseRadarBox({ station }: { station: RadioStationDetail }) {
  const { data } = useNewReleaseRadar({ stationId: station.id, limit: RADAR_LIMIT })
  const releases = data?.releases ?? []
  if (releases.length === 0) return null

  return (
    <section aria-label="New release radar">
      <SectionHeader title={`New release radar — ${station.name}`} />
      <ul className="space-y-2 text-sm">
        {releases.map((entry, i) => (
          <li key={`${entry.artist_name}-${entry.album_title}-${i}`}>
            <div className="leading-snug">
              {entry.artist_slug ? (
                <Link
                  href={`/artists/${entry.artist_slug}`}
                  className="font-medium text-foreground hover:text-primary transition-colors"
                >
                  {entry.artist_name}
                </Link>
              ) : (
                <span className="font-medium">{entry.artist_name}</span>
              )}
              {entry.album_title && (
                <>
                  <span className="text-muted-foreground"> — </span>
                  {entry.release_slug ? (
                    <Link
                      href={`/releases/${entry.release_slug}`}
                      className="text-foreground hover:text-primary transition-colors"
                    >
                      {entry.album_title}
                    </Link>
                  ) : (
                    <span>{entry.album_title}</span>
                  )}
                </>
              )}
            </div>
            <div className="font-mono text-xs text-muted-foreground">
              {entry.label_name && (
                <>
                  {entry.label_slug ? (
                    <Link
                      href={`/labels/${entry.label_slug}`}
                      className="hover:text-foreground transition-colors"
                    >
                      {entry.label_name}
                    </Link>
                  ) : (
                    entry.label_name
                  )}
                  {' · '}
                </>
              )}
              {pluralize(entry.play_count, 'play')}
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
