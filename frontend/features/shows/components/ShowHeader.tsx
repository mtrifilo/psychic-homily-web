'use client'

import Link from 'next/link'
import { ExternalLink, MapPin } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { formatShowDate, formatShowTime, formatPrice } from '@/lib/utils/formatters'
import { ShowAddToCalendar } from './ShowAddToCalendar'
import type { ArtistResponse, SetType, ShowResponse } from '../types'

/**
 * Bill order lives in `show_artists.position`. Every backend read path already
 * sorts by it (`buildShowResponse`, `loadShowArtistResponses`), so this is a
 * defensive re-assertion against a caller, cache layer, or future query handing
 * us a different order.
 *
 * Ties are possible: `idx_show_artists_position` is a plain index, so nothing
 * enforces one position per show, and rows written outside the create path
 * (backfills, seeds) can share position 0. The backend's `ORDER BY position
 * ASC` has no tiebreaker, so Postgres may order tied rows differently between
 * requests. Break the tie on `id` so the rendered bill is at least
 * deterministic client-side.
 */
function byBillPosition(a: ArtistResponse, b: ArtistResponse): number {
  return a.position - b.position || a.id - b.id
}

/**
 * Support-line annotations, keyed by `set_type`.
 *
 * `opener` is deliberately absent. It reads like a distinguishing role but is
 * really the backend's default for "not the headliner" — `associateArtists`
 * hardcodes it for every non-headliner, and the discovery fallback does the
 * same — so annotating it would append "(opener)" to nearly every support act
 * on nearly every bill. Labelling it only becomes meaningful once `set_type`
 * carries real semantics — a richer vocabulary plus a backfill. Don't re-add
 * it here before that lands.
 *
 * A `Map` rather than an object literal on purpose: `set_type` is a bare
 * `string` on the wire (see `types/api.d.ts`) over an unconstrained VARCHAR
 * column, and the `SetType` union here is a hand-maintained narrowing that
 * nothing validates at runtime. An object lookup would walk the prototype
 * chain, so a stored `__proto__` would resolve to `Object.prototype` and
 * crash the server render; `Map.get` has no such hole.
 */
const SUPPORT_SET_TYPE_LABELS = new Map<string, string>([
  ['special_guest', 'special guest'],
])

function SupportSetTypeLabel({ setType }: { setType: SetType }) {
  const label = SUPPORT_SET_TYPE_LABELS.get(setType)
  if (!label) return null
  return (
    <span className="text-sm text-muted-foreground/70 italic"> ({label})</span>
  )
}

interface ShowHeaderProps {
  show: ShowResponse
  /**
   * Action cluster rendered on the right side of the header (desktop) or
   * below the artist/venue block (mobile). Typically a `<ShowActions />`.
   */
  actions?: React.ReactNode
}

/**
 * The reserved left column of the show page: a flyer at its native aspect
 * ratio, uncropped.
 *
 * A plain plate today. The flyer itself, its provenance caption and its report
 * affordance are the next wave's work, and drawing a dashed "image goes here"
 * box or a fake caption would be promising UI that does not exist. What this
 * DOES do is hold the column open so the two-column reading order is real and
 * reviewable now rather than arriving as a surprise reflow later.
 *
 * Its column is hidden below `md` (see the caller): a tall empty plate above
 * the bill would push every word of the show off a phone screen, and the
 * mock's two-column grammar only exists at desktop width anyway.
 */
function ShowFlyerPlate() {
  return (
    <div
      aria-hidden="true"
      data-testid="show-flyer-plate"
      className="aspect-[4/5] w-full rounded-sm border border-border/60 bg-muted/40"
    />
  )
}

/**
 * ShowDetail-specific header block. Owns the bill-position artist rendering
 * (headliners as h1, support as "w/ ..." row), venue prominence block
 * (name link + MapPin + "see more shows at {venue}" link), date + sold-out
 * badge row, show meta row (time / price / age), ticket URL CTA, and
 * description paragraph.
 *
 * Laid out as the mock's two columns: the flyer plate on the left, and on the
 * right the module slots in reading order (header block, venue, ticket and
 * actions, attendance). The slots are marked in the markup because their
 * ORDER is the design decision; what goes inside each one is still being
 * filled in wave by wave, and a later wave should have somewhere obvious to
 * put its module rather than choosing a new position for it.
 *
 * This intentionally diverges from the generic `EntityHeader` — the bill
 * position semantics (`set_type`) and the co-primary venue entity don't
 * fit into `EntityHeader`'s single-string `title` / subtitle-badge shape.
 * See `docs/research/entity-detail-layout-migration.md` for rationale.
 */
export function ShowHeader({ show, actions }: ShowHeaderProps) {
  const venue = show.venues[0]
  // Sort the whole bill first so every downstream slice — including the
  // `artists[0]` / `artists.slice(1)` fallback below — is position-ordered.
  const artists = [...show.artists].sort(byBillPosition)

  // Trimmed once here: the API can hand back a whitespace-only address, which
  // would otherwise pass the truthiness check and render a blank indented line.
  const venueAddress = venue?.address?.trim()

  const headliners = artists.filter(
    a => a.set_type === 'headliner' || a.is_headliner === true
  )
  const support = artists.filter(
    a => a.set_type !== 'headliner' && a.is_headliner !== true
  )
  const effectiveHeadliners =
    headliners.length > 0 ? headliners : artists.length > 0 ? [artists[0]] : []
  const effectiveSupport = headliners.length > 0 ? support : artists.slice(1)

  return (
    <div className="grid grid-cols-1 gap-6 md:grid-cols-[minmax(0,18rem)_minmax(0,1fr)] md:gap-8">
      {/* SLOT: flyer plate (+ its provenance caption, a later wave). The whole
          column is hidden below `md`, not just its contents: a display:none
          grid item generates no row and no gap, so a phone gets the bill at
          the top of the page rather than a screen of reserved plate. */}
      <div className="hidden min-w-0 md:block">
        <ShowFlyerPlate />
      </div>

      <div className="min-w-0">
        {/* SLOT: header block. Date, bill, context. */}
        {/* Date and Status Badges */}
        <div className="flex items-center gap-2 mb-2">
          <span className="text-lg font-bold text-primary">
            {formatShowDate(show.event_date, show.state, false, show.venues?.[0]?.timezone)}
          </span>
          {show.is_sold_out && (
            <Badge
              variant="secondary"
              className="text-xs font-semibold bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400"
            >
              SOLD OUT
            </Badge>
          )}
        </div>

        {/* Artists — grouped by billing */}
        <h1 className="text-2xl md:text-3xl font-bold leading-8 md:leading-9">
          {effectiveHeadliners.map((artist, index) => (
            <span key={artist.id}>
              {index > 0 && (
                <span className="text-muted-foreground/60 font-normal">
                  {' '}
                  &bull;{' '}
                </span>
              )}
              {artist.slug ? (
                <Link
                  href={`/artists/${artist.slug}`}
                  className="hover:text-primary transition-colors"
                >
                  {artist.name}
                </Link>
              ) : (
                <span>{artist.name}</span>
              )}
            </span>
          ))}
        </h1>
        {effectiveSupport.length > 0 && (
          <div className="text-lg text-muted-foreground mt-1">
            <span className="italic">w/</span>{' '}
            {effectiveSupport.map((artist, index) => (
              <span key={artist.id}>
                {index > 0 && (
                  <span className="text-muted-foreground/50">, </span>
                )}
                {artist.slug ? (
                  <Link
                    href={`/artists/${artist.slug}`}
                    className="hover:text-primary/80 transition-colors"
                  >
                    {artist.name}
                  </Link>
                ) : (
                  <span>{artist.name}</span>
                )}
                <SupportSetTypeLabel setType={artist.set_type} />
              </span>
            ))}
          </div>
        )}

        {/* SLOT: venue module. Refined by the venue-module wave; today it is
            the name link, city/state, street address and the "more shows at"
            link that already lived here. */}
        {venue && (
          <div className="mt-4">
            {venue.slug ? (
              <Link
                href={`/venues/${venue.slug}`}
                className="text-lg text-primary/80 hover:text-primary font-medium transition-colors"
              >
                {venue.name}
              </Link>
            ) : (
              <span className="text-lg text-primary/80 font-medium">
                {venue.name}
              </span>
            )}
            <div className="flex items-center gap-1 text-muted-foreground mt-1">
              <MapPin className="h-4 w-4" />
              <span>
                {venue.city}, {venue.state}
              </span>
            </div>
            {/* Street address — plain text, no maps link. `pl-5` (icon w-4 +
                gap-1) hangs it under the city/state text so the two read as one
                location group. */}
            {venueAddress && (
              <div
                data-testid="venue-address"
                className="pl-5 text-sm text-muted-foreground"
              >
                {venueAddress}
              </div>
            )}
            {venue.slug && (
              <Link
                href={`/venues/${venue.slug}`}
                className="inline-block text-sm text-muted-foreground hover:text-primary transition-colors mt-1"
              >
                See more shows at {venue.name} &rarr;
              </Link>
            )}
          </div>
        )}

        {/* SLOT: ticket and action block. The when/what-it-costs line and
            every verb a reader can act on, together in one band under the
            venue, as the mock has them. The action cluster used to float in a
            right-hand column of the header; it was moved, not rewired, so the
            calendar/save coupling it carries is untouched. */}
        <div className="mt-4 border-t border-border/60 pt-4">
          {/* The calendar affordance sits here with the when-info (event-page
              convention), not in the social action cluster. */}
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
            <span>{formatShowTime(show.event_date, show.state, show.venues?.[0]?.timezone)}</span>
            {show.price != null && <span>{formatPrice(show.price)}</span>}
            {show.age_requirement && <span>{show.age_requirement}</span>}
            <ShowAddToCalendar show={show} />
          </div>

          {/* Ticket URL */}
          {show.ticket_url && (
            <div className="mt-3">
              <a
                href={
                  show.ticket_url.startsWith('http')
                    ? show.ticket_url
                    : `https://${show.ticket_url}`
                }
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
              >
                Buy Tickets
                <ExternalLink className="h-3.5 w-3.5" />
              </a>
            </div>
          )}

          {actions && (
            <div className="mt-3 flex flex-col items-start gap-2">{actions}</div>
          )}
        </div>

        {/* SLOT: attendance. Going / interested / "I was there" counts land
            here, between the actions and the fold. Deliberately empty: the
            counts are designed but not built, and reserving visible blank
            space for them would read as a broken module. */}

        {/* Description */}
        {show.description && (
          <p className="mt-4 text-muted-foreground">{show.description}</p>
        )}
      </div>
    </div>
  )
}
