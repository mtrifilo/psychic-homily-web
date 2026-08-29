'use client'

import { useMemo } from 'react'
import type { VenueWithShowCount } from '@/features/venues/types'
import { formatTimeAgo } from '@/lib/formatTimeAgo'
import { GENRE_FAMILIES } from '../genreFamilies'
import {
  CITY_RAIL_WIDTH_PX,
  cityContributionCounts,
  cityContributionSegments,
  cityDataUpdatedAt,
  cityAllAgesTagDetermined,
  cityGenreFamilies,
  cityHasAllAgesVenue,
  cityRailStats,
  emptyRailReason,
  type EmptyRailReason,
  formatNextShowDate,
  nextShowBill,
  venueLocalityLabel,
  venuesSpanMetro,
  type CityVenueFilters,
} from '../cityView'

/** Ties the all-ages chip to its "at least sometimes" caveat via aria-describedby. */
const ALL_AGES_CAVEAT_ID = 'atlas-rail-all-ages-caveat'

const GENRE_LABEL_BY_KEY: ReadonlyMap<string, string> = new Map(
  // Lowercased: the rail's meta line is running mono text ("… · punk &
  // hardcore"), not a legend entry, so the legend's title case would shout.
  GENRE_FAMILIES.map((f) => [f.key, f.label.toLowerCase()]),
)

interface VenueRailProps {
  /** Display name of the city the camera is on, e.g. "Austin, TX". */
  cityLabel: string
  /**
   * The metro's principal city on its own, e.g. "Austin". Rows in any OTHER
   * city of the metro print theirs (PSY-1574); rows in this one don't, since
   * the header already says it.
   */
  principalCity: string
  /** Venues AFTER filtering — the same array the map pins. */
  venues: readonly VenueWithShowCount[]
  /** Venues BEFORE filtering, for the header stats and the genre menu. */
  allVenues: readonly VenueWithShowCount[]
  /** Scene-level roster size ("35 LOCAL ARTISTS"); undefined while loading. */
  localArtistCount?: number
  /**
   * How many venues the city has in total, from the API. Greater than
   * `allVenues.length` means the fetch cap truncated the list, which the rail
   * must SAY rather than quietly present a partial city as the whole one.
   */
  totalVenueCount?: number
  loading?: boolean
  /**
   * The venue request failed and retries are exhausted. Distinct from
   * `loading` and from an empty city: a request that never landed leaves the
   * same zero rows a genuinely empty city does, and "No venues listed here
   * yet" is then a flat lie about the place. Public reads here really do fail
   * (the anonymous per-IP limiter 429s them), so the rail has to be able to
   * say so.
   */
  fetchFailed?: boolean
  filters: CityVenueFilters
  onFiltersChange: (filters: CityVenueFilters) => void
  selectedVenueId: number | null
  onVenueSelect: (venueId: number) => void
  /** "← globe": fly the camera back out to the globe view. */
  onBackToGlobe: () => void
}

/**
 * The Atlas city view's left rail (PSY-1539) — a dense list of the city's
 * venues, synced with the pins beside it.
 *
 * It sits BESIDE the map, not over it: the map pane is narrowed by exactly
 * this rail's width. That is a licensing constraint as much as a layout one —
 * the map's bottom-left OpenStreetMap attribution is required by the ODbL, and
 * PSY-1543's review found a full-height right-edge panel hiding it outright.
 *
 * The rail renders `venues` (already filtered) and derives its header counts
 * from `allVenues`, so the header describes the city while the list shows the
 * current cut of it.
 */
export function VenueRail({
  cityLabel,
  principalCity,
  venues,
  allVenues,
  localArtistCount,
  totalVenueCount,
  loading = false,
  fetchFailed = false,
  filters,
  onFiltersChange,
  selectedVenueId,
  onVenueSelect,
  onBackToGlobe,
}: VenueRailProps) {
  const stats = useMemo(() => cityRailStats(allVenues), [allVenues])
  const genreFamilies = useMemo(() => cityGenreFamilies(allVenues), [allVenues])
  const updatedAt = useMemo(() => cityDataUpdatedAt(allVenues), [allVenues])
  const spansMetro = useMemo(
    () => venuesSpanMetro(allVenues, principalCity),
    [allVenues, principalCity],
  )
  const contributionSegments = useMemo(
    () => cityContributionSegments(cityContributionCounts(allVenues)),
    [allVenues],
  )
  // The fetch cap bit, so `allVenues` is one busiest-first page rather than the
  // whole metro. Three things read it: the "showing the N busiest of M" line,
  // the empty state (which must not generalise from a partial city), and the
  // active-filter caveat below.
  const listTruncated =
    totalVenueCount !== undefined && totalVenueCount > allVenues.length

  // All over the UNFILTERED list on purpose: together they separate "no venue
  // here carries the tag" from "your filters excluded everything" from "the tag
  // lookup never answered". emptyRailReason owns the precedence between them.
  const emptyReason = useMemo(
    () =>
      emptyRailReason({
        fetchFailed,
        cityEmpty: allVenues.length === 0,
        allAgesOnly: filters.allAgesOnly,
        hasAllAgesVenue: cityHasAllAgesVenue(allVenues),
        tagDetermined: cityAllAgesTagDetermined(allVenues),
        listTruncated,
      }),
    [allVenues, fetchFailed, filters.allAgesOnly, listTruncated],
  )

  const activeGenreLabel = filters.genreFamily
    ? (GENRE_LABEL_BY_KEY.get(filters.genreFamily) ?? 'All genres')
    : 'All genres'

  return (
    <aside
      aria-label={`Venues in ${cityLabel}`}
      data-testid="atlas-venue-rail"
      /* Width from the shared constant, not a Tailwind literal: AtlasGlobe
         subtracts the SAME number to size the map canvas, and two
         independently-editable copies of it would desync the two panes. */
      style={{ width: CITY_RAIL_WIDTH_PX }}
      className="flex h-full shrink-0 flex-col border-r border-border bg-card"
    >
      <header className="border-b border-border px-4 pb-3 pt-4">
        <div className="flex items-baseline gap-3">
          <h2 className="text-xl font-semibold text-foreground">{cityLabel}</h2>
          <button
            type="button"
            onClick={onBackToGlobe}
            className="font-mono text-xs text-muted-foreground underline-offset-4 transition-colors hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            ← globe
          </button>
        </div>

        <p className="mt-2 font-mono text-[11px] uppercase leading-4 tracking-wide text-muted-foreground">
          {/* "metro venues" whenever the rows reach past the principal city
              (PSY-1574). The heading above names ONE city; a flat "12 venues"
              under it would read as a claim about Phoenix proper while the
              list also holds Tempe and Mesa. The last stat sums the venues'
              rolling `shows_this_week`, hence "in the next 7 days" — see
              `VenueWithShowCount.shows_this_week`. Same phrasing as the pulse
              band and the globe tooltip; the "in" keeps "9 IN THE NEXT 7 DAYS"
              from setting two numerals side by side in this uppercase-mono
              strip, where every other segment reads "<number> <noun>". */}
          {stats.venueCount} {spansMetro ? 'metro ' : ''}
          {stats.venueCount === 1 ? 'venue' : 'venues'} · {stats.upcomingCount}{' '}
          upcoming · {stats.thisWeekCount} in the next 7 days
          {localArtistCount !== undefined && (
            <> · {localArtistCount} local artists</>
          )}
        </p>

        {/* The list is one page deep. A city with more venues than the cap
            would otherwise read as if it had exactly the cap — say so instead.
            The API sorts busiest-first, so "busiest" is accurate. */}
        {listTruncated && (
          <p className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground">
            showing the {allVenues.length} busiest of {totalVenueCount}
          </p>
        )}

        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <FilterChip
            active={filters.thisWeekOnly}
            onClick={() =>
              onFiltersChange({ ...filters, thisWeekOnly: !filters.thisWeekOnly })
            }
          >
            Next 7 days
          </FilterChip>

          {/* A native select styled as a chip: the menu is a list of the
              families actually present, so every option leads somewhere.
              The select comes FIRST in the DOM (and sits on top via z-10) so
              the visible chip can be its Tailwind `peer` — the select itself
              is transparent, so its own focus ring is invisible and a
              keyboard user would otherwise get no focus indicator at all. */}
          <span className="relative inline-flex">
            <select
              aria-label="Filter venues by genre"
              value={filters.genreFamily ?? ''}
              onChange={(e) =>
                onFiltersChange({
                  ...filters,
                  genreFamily: e.target.value === '' ? null : e.target.value,
                })
              }
              className="peer absolute inset-0 z-10 cursor-pointer opacity-0"
            >
              <option value="">All genres</option>
              {genreFamilies.map((f) => (
                <option key={f.key} value={f.key}>
                  {f.label}
                </option>
              ))}
            </select>
            <span
              aria-hidden="true"
              className={`${chipClass(filters.genreFamily !== null)} peer-focus-visible:ring-2 peer-focus-visible:ring-ring`}
            >
              {activeGenreLabel} ⌄
            </span>
          </span>

          {/* "All-ages shows", not the mock's "All ages" (user decision,
              PSY-1573). The tag-vs-column question is settled the other way
              from what this comment used to say: the filter reads the canonical
              `all-ages` TAG, not the free-text `venues.age_policy` column.
              age_policy is the HOUSE DEFAULT, and the traveler's question is
              "can I get in to something here", which a 21+ room booking an
              all-ages matinee answers yes to and its column answers no to.

              Hence the copy. The tag means the room hosts all-ages shows AT
              LEAST SOMETIMES, so the chip names the shows rather than the room:
              a bare "All ages" beside a venue name reads as a promise about
              every night there, which nobody has made. */}
          <FilterChip
            active={filters.allAgesOnly}
            title="Venues that host all-ages shows at least sometimes"
            /* The caveat below is the chip's DESCRIPTION, not decoration.
               `title` alone leaves a screen-reader user with just the label
               "All-ages shows" (most SRs drop title on an already-labelled
               control), so the same sentence sighted users get is wired here
               and rendered unconditionally — a description has to exist before
               activation, not appear after it. */
            describedById={ALL_AGES_CAVEAT_ID}
            onClick={() =>
              onFiltersChange({ ...filters, allAgesOnly: !filters.allAgesOnly })
            }
          >
            All-ages shows
          </FilterChip>
          {/* Still a disabled placeholder, exactly as the mock draws it —
              record stores are a later chapter of the travel-mode project, and
              the Atlas has no non-venue places on it at all yet. */}
          <FilterChip disabled title="Record stores aren’t on the Atlas yet">
            Record stores
          </FilterChip>
        </div>

        {/* The caveat has to be VISIBLE while the filter is on, not parked in
            the chip's `title`. A tooltip never opens on touch — the Atlas
            travel-mode audience's likeliest device — and most screen readers
            skip `title` on a control that already has a text label. Without
            this line, a phone user narrowing the list would see nothing but
            "All-ages shows" above it, which reads as a promise about every
            night at every room listed.

            ALWAYS RENDERED, and merely hidden while the chip is off: it is the
            chip's aria-describedby target, and a description that only exists
            after activation is not a description. Visually it still appears
            only when the filter is on, so the chip row stays quiet by default.

            Copy agreed with the user directly (2026-08-27), NOT taken from the
            Figma mock — the mock draws neither this line nor the "All-ages
            shows" label, so do not "restore" either to match board 01. */}
        <p
          id={ALL_AGES_CAVEAT_ID}
          className={
            filters.allAgesOnly
              ? 'mt-2 font-mono text-[11px] leading-4 text-muted-foreground'
              : 'sr-only'
          }
        >
          rooms that host all-ages shows at least sometimes. check the
          individual show.
        </p>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {loading && venues.length === 0 ? (
          <p className="px-4 py-6 text-sm text-muted-foreground">
            Loading venues…
          </p>
        ) : venues.length === 0 ? (
          <EmptyRail reason={emptyReason} shownCount={allVenues.length} />
        ) : (
          <ul>
            {venues.map((venue) => (
              <li key={venue.id}>
                <VenueRow
                  venue={venue}
                  principalCity={principalCity}
                  selected={venue.id === selectedVenueId}
                  onSelect={() => onVenueSelect(venue.id)}
                />
              </li>
            ))}
          </ul>
        )}
      </div>

      <footer className="border-t border-border px-4 py-3">
        {/* Provenance for the city (PSY-1542). Timestamp = newest updated_at
            across the listed venues; edits and confirmations are sums over
            them.

            The mock's "by M contributors" is deliberately absent HERE and
            present on the venue panel. Each venue reports its own DISTINCT
            contributor count, and distinct counts don't add up — one person
            maintaining three venues would be counted three times. The panel's
            number is exact because it is scoped to one venue; a city-wide one
            would be a plausible-looking overstatement. See
            cityContributionCounts.

            The mock's "✓ Confirm this list is current" is deliberately NOT
            built (user decision, 2026-07-27). There is no "list" object to
            write to — scenes are computed views with no table — and making it
            a bulk confirm of every listed venue would make each confirmation
            far weaker evidence than the deliberate per-venue one in the panel.
            Confirming happens where you can actually vouch for the place. */}
        <p
          data-testid="rail-provenance"
          className="font-mono text-[11px] leading-4 text-muted-foreground"
        >
          {/* Full-strength muted, not a dimmed variant: at 11px an opacity
              step lands under the 4.5:1 contrast floor. The line's hierarchy
              comes from the VALUE being brighter, not the label dimmer. */}
          <span className="text-muted-foreground">DATA</span>{' '}
          {updatedAt ? `updated ${formatTimeAgo(updatedAt)}` : 'no update recorded'}
          {contributionSegments.map((segment) => (
            <span key={segment}> · {segment}</span>
          ))}
        </p>
      </footer>
    </aside>
  )
}

/**
 * What the rail says when it has no rows to show.
 *
 * Five facts, five sentences, chosen by ONE discriminated value rather than a
 * set of booleans this component could combine into a contradiction. A filter
 * that matches nothing has to say WHY, or a working feature reads as a broken
 * one — and "no venues match these filters" is a claim about a CITY, which is
 * the wrong claim when the real answer is about our data. The all-ages tag has
 * near-zero coverage in production (nothing seeds production data with it, and
 * the dev seed applies it only to its own exemplar venues), so that wrong
 * reading would be the common one. emptyRailReason owns the precedence.
 *
 * None of these invite the reader to go add the tag. There is nowhere to do it:
 * VenueDetail mounts EntityTagList but not the AddTagDialog + "[Add tag]"
 * affordance that ArtistDetail and its peers pair with it, and EntityTagList
 * renders nothing at all for an untagged entity. Pointing at a control that
 * does not exist would make this empty state the dishonest thing it was written
 * to replace. Mounting that affordance on venues needs its own ticket, which is
 * not yet filed.
 */
function EmptyRail({
  reason,
  shownCount,
}: {
  reason: EmptyRailReason
  shownCount: number
}) {
  if (reason === 'fetch-failed') {
    return (
      <p className="px-4 py-6 text-sm text-muted-foreground">
        Couldn’t load venues here. Try again in a moment.
      </p>
    )
  }
  if (reason === 'city-empty') {
    return (
      <p className="px-4 py-6 text-sm text-muted-foreground">
        No venues listed here yet.
      </p>
    )
  }
  if (reason === 'all-ages-undetermined') {
    return (
      <p className="px-4 py-6 text-sm text-muted-foreground">
        Couldn’t check which venues host all-ages shows just now. Clear the
        filter to see the full list.
      </p>
    )
  }
  if (reason === 'all-ages-unseeded' || reason === 'all-ages-unseeded-in-view') {
    return (
      <div className="px-4 py-6 text-sm text-muted-foreground">
        {/* Says "not yet recorded", never "there are none". The venues exist;
            the tag is what's missing, so the absence is a gap in our data.
            The truncated variant additionally admits how far it looked, rather
            than generalising one busiest-first page to a whole metro. */}
        <p>
          {reason === 'all-ages-unseeded-in-view'
            ? `No venue among the ${shownCount} busiest here is tagged for all-ages shows yet.`
            : 'No venue here is tagged for all-ages shows yet.'}
        </p>
        <p className="mt-2">It doesn’t mean the city has none.</p>
      </div>
    )
  }
  return (
    <p className="px-4 py-6 text-sm text-muted-foreground">
      No venues match these filters.
    </p>
  )
}

function chipClass(active: boolean, disabled = false): string {
  const base =
    'inline-flex items-center rounded-sm px-2 py-0.5 font-mono text-[11px] leading-4 transition-colors'
  if (disabled) return `${base} bg-muted/50 text-muted-foreground/50`
  if (active) return `${base} bg-primary text-background`
  return `${base} bg-muted text-foreground hover:bg-muted/70`
}

function FilterChip({
  children,
  active = false,
  disabled = false,
  title,
  describedById,
  onClick,
}: {
  children: React.ReactNode
  active?: boolean
  disabled?: boolean
  title?: string
  /**
   * Id of an element describing this chip, wired as `aria-describedby`.
   * `title` does NOT reach most screen readers on a control that already has a
   * text label, so any caveat a chip's label depends on belongs here too.
   */
  describedById?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      title={title}
      aria-describedby={describedById}
      aria-pressed={disabled ? undefined : active}
      onClick={onClick}
      className={`${chipClass(active, disabled)} ${
        disabled
          ? 'cursor-not-allowed'
          : 'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
      }`}
    >
      {children}
    </button>
  )
}

function VenueRow({
  venue,
  principalCity,
  selected,
  onSelect,
}: {
  venue: VenueWithShowCount
  principalCity: string
  selected: boolean
  onSelect: () => void
}) {
  const nextDate = formatNextShowDate(venue.next_show_date)
  const bill = nextShowBill(venue)
  const genre = venue.dominant_genre
    ? GENRE_LABEL_BY_KEY.get(venue.dominant_genre)
    : undefined
  const locality = venueLocalityLabel(venue, principalCity)

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected ? 'true' : undefined}
      className={`w-full border-b border-border/60 px-4 py-2.5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring ${
        selected ? 'bg-muted/60' : 'hover:bg-muted/30'
      }`}
    >
      <div className="flex items-baseline justify-between gap-3">
        <span className="truncate text-sm text-foreground">{venue.name}</span>
        <span className="shrink-0 font-mono text-xs text-primary">
          {venue.upcoming_show_count} upcoming
        </span>
      </div>
      <p className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground">
        {/* The rail is scoped to the METRO (PSY-1574), so a row can sit in a
            city the header doesn't name and outside the current frame. Leading
            the meta line with that city is what keeps such a row from reading
            as a mistake. Uppercase, matching the NEXT/DATA label convention —
            it's a place stamp, not prose — and absent for the principal city,
            where it would be noise on every row. */}
        {locality && (
          <>
            <span className="uppercase tracking-wide text-foreground/70">
              {locality}
            </span>{' '}
            ·{' '}
          </>
        )}
        {nextDate ? (
          <>
            {/* See the DATA label: full-strength muted for contrast; the date
                carries the emphasis by being brighter. */}
            <span className="text-muted-foreground">NEXT</span>{' '}
            <span className="text-foreground/70">{nextDate}</span>
            {bill && <> · {bill}</>}
          </>
        ) : (
          <>nothing on the calendar</>
        )}
        {genre && <> · {genre}</>}
      </p>
    </button>
  )
}
