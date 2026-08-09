'use client'

import { Fragment, useCallback, useState } from 'react'
import Link from 'next/link'
import { ExternalLink, MapPin } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { formatLocation } from '@/lib/formatLocation'
import { formatShowDate, formatShowTime, formatPrice } from '@/lib/utils/formatters'
import { ShowAddToCalendar } from './ShowAddToCalendar'
import { ShowFlyerPlate } from './ShowFlyerPlate'
import { flyerCredit, flyerImageSrc } from './showFlyer'
import { showTimingInput, splitBill } from '../utils'
import type {
  ArtistResponse,
  SetType,
  ShowArtistLabel,
  ShowResponse,
} from '../types'

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
 * `opener` is deliberately absent, and the reason has CHANGED without the
 * behaviour changing. It used to be the backend's default for "not the
 * headliner", so annotating it would have marked nearly every support act on
 * nearly every bill. That is no longer true: `set_type` is curated and
 * authoritative, and the neutral default is now `performer` ("on the bill,
 * slot unknown"). What keeps `opener` out today is the DESIGN: the locked show
 * mock renders no role labels on the bill at all. Support acts are `w/` lines
 * and nothing more. Adding role annotations back is a design decision, not a
 * data-quality one, so it needs a call rather than a patch here.
 *
 * `special_guest` predates that mock and still renders. Left alone on purpose:
 * removing a shipped annotation is as much a design change as adding one.
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

/**
 * The labels an act records for, as one bracketed group: `[Epic]`,
 * `[Jealous Butcher · Dead Oceans]`.
 *
 * ONE bracket pair around the whole group, middots inside it, not a bracket
 * per label. Two labels are a single fact about one band; `[A] [B]` reads as
 * two separate affordances, which is what {@link BracketLink} means elsewhere
 * on the page. That is also why this is hand-rolled rather than composed from
 * BracketLink: the brackets here are annotation, and each NAME inside is its
 * own link.
 *
 * The brackets and the middot are decoration, so they are `aria-hidden` and a
 * screen reader gets the word "on" instead: "Modest Mouse on Epic". Without
 * that word the punctuation was the ONLY thing saying "Epic" is a different
 * kind of fact from "Modest Mouse", and hiding it left three proper nouns in
 * a row. Note where the spaces sit: OUTSIDE the hidden spans. A space inside
 * an `aria-hidden` subtree is removed along with it, which runs the label
 * names together as "Jealous ButcherDead Oceans".
 *
 * A missing `slug` renders as plain text: `labels.slug` is nullable in the
 * database and the backend flattens null to "", which would otherwise build a
 * link to `/labels/`.
 */
function BillLabels({
  labels,
  className,
}: {
  labels?: ShowArtistLabel[]
  className?: string
}) {
  if (!labels || labels.length === 0) return null
  return (
    <>
      {' '}
      <span className={cn('font-mono font-normal text-primary', className)}>
        {/* The space after the connective is its own text node, not a trailing
            space inside the span: accessible-name computation trims each node,
            which is how "on Epic" becomes "onEpic". Two adjacent whitespace
            nodes still paint as one space. */}
        <span className="sr-only">on</span>{' '}
        <span aria-hidden="true">[</span>
        {labels.map((label, index) => (
          <span key={label.id}>
            {index > 0 && (
              <>
                {' '}
                <span aria-hidden="true" className="text-primary/60">
                  &middot;
                </span>{' '}
              </>
            )}
            {label.slug ? (
              <Link
                href={`/labels/${label.slug}`}
                className="hover:underline focus-visible:underline"
              >
                {label.name}
              </Link>
            ) : (
              <span>{label.name}</span>
            )}
          </span>
        ))}
        <span aria-hidden="true">]</span>
      </span>
    </>
  )
}

/**
 * Where an act is from, inline after its labels: `Issaquah, WA`,
 * `Melbourne, Australia`.
 *
 * Delegates to `formatLocation` so the bill obeys the same locked display rule
 * as every other surface: country included UNLESS the state is set and the
 * country is USA/US. An act with nothing placeable renders NO segment. The
 * helper's "Location Unknown" placeholder is designed to stand alone in a
 * location field, and "Modest Mouse [Epic] Location Unknown" states something
 * the bill was not asked to state.
 *
 * Carries the same kind of screen-reader-only connective as {@link BillLabels}
 * and for the same reason: visually a city sits in its own typographic slot,
 * but read aloud it is one more proper noun unless something says "from".
 */
function BillHometown({
  artist,
  className,
}: {
  artist: ArtistResponse
  className?: string
}) {
  // Judged on the PARTS, not on the formatted string. Comparing the result to
  // `LOCATION_UNKNOWN` would also silence an artist whose city is literally
  // "Location Unknown", which is exactly the placeholder an extraction run
  // writes when it does not know.
  const hasPlaceableLocation = [
    artist.city,
    artist.state,
    artist.country,
  ].some(part => part?.trim())
  if (!hasPlaceableLocation) return null
  const hometown = formatLocation({
    city: artist.city,
    state: artist.state,
    country: artist.country,
  })
  return (
    <>
      {' '}
      <span className={cn('font-normal text-muted-foreground', className)}>
        {/* Space outside the span, for the same reason as BillLabels' "on". */}
        <span className="sr-only">from</span> {hometown}
      </span>
    </>
  )
}

interface ShowHeaderProps {
  show: ShowResponse
  /**
   * Action cluster rendered at the foot of the ticket block, under the price
   * and ticket link, at every width. Typically a `<ShowActions />`.
   */
  actions?: React.ReactNode
}

/**
 * ShowDetail-specific header block. Owns the bill-position artist rendering
 * (headliners as h1, support as "w/ ..." row), venue prominence block
 * (name link + MapPin + "see more shows at {venue}" link), date + sold-out
 * badge row, show meta row (time / price / age), ticket URL CTA, and
 * description paragraph.
 *
 * Laid out as the mock's two columns WHEN THERE IS A FLYER: the plate on the
 * left, and on the right the module slots in reading order (header block,
 * venue, ticket and actions, attendance). A show with no usable flyer drops
 * the left column entirely and the typeset content takes the full width. The
 * grid is a response to the data, not a fixed frame. The slots are marked in
 * the markup because their ORDER is the design decision; what goes inside each
 * one is still being filled in wave by wave, and a later wave should have
 * somewhere obvious to put its module rather than choosing a new position for
 * it.
 *
 * This intentionally diverges from the generic `EntityHeader` — the bill
 * position semantics (`set_type`) and the co-primary venue entity don't
 * fit into `EntityHeader`'s single-string `title` / subtitle-badge shape.
 * See `docs/research/entity-detail-layout-migration.md` for rationale.
 */
export function ShowHeader({ show, actions }: ShowHeaderProps) {
  // A flyer URL that 404s, or points at a host that blocks hotlinking, is the
  // same situation as no flyer at all, so it collapses the column the same
  // way rather than leaving a broken-image glyph in a reserved gutter.
  //
  // The failed URL is stored, not a boolean: an admin can fix `image_url` from
  // the edit drawer on this very page, and the live query then re-renders this
  // component with a new src, which a boolean would keep suppressed until a
  // reload. One slot, so it only forgives FORWARD: switching back to a URL
  // that failed earlier in this mount stays suppressed even if the host has
  // since recovered. Refreshing fixes that, and it is not worth a set.
  const [failedFlyerSrc, setFailedFlyerSrc] = useState<string | null>(null)
  const candidateFlyerSrc = flyerImageSrc(show)
  // ONE value, not a boolean plus a URL. The grid template below and the plate
  // at the foot of this component have to agree about whether there is a
  // flyer; two expressions that must stay in step is an invariant somebody
  // eventually breaks, and the failure mode is a two-column desktop layout
  // with an empty left column.
  const flyerSrc =
    candidateFlyerSrc !== failedFlyerSrc ? candidateFlyerSrc : null
  // Stable per URL: the plate holds it in a callback ref, and an identity that
  // changed every render would detach and re-attach that ref every render.
  const handleFlyerError = useCallback(
    () => setFailedFlyerSrc(candidateFlyerSrc),
    [candidateFlyerSrc]
  )

  const venue = show.venues[0]
  // The same zone the status stripe above this block is judged on. They render
  // a few hundred pixels apart, so a page that resolved the venue's calendar
  // two ways could print two different dates in one screenshot.
  const timing = showTimingInput(show)
  // Sort the whole bill first so every downstream slice — including the
  // `artists[0]` / `artists.slice(1)` fallback below — is position-ordered.
  const artists = [...show.artists].sort(byBillPosition)

  // Trimmed once here: the API can hand back a whitespace-only address, which
  // would otherwise pass the truthiness check and render a blank indented line.
  const venueAddress = venue?.address?.trim()

  const { headliners: effectiveHeadliners, support: effectiveSupport } =
    splitBill(artists)

  return (
    <div
      className={cn(
        'grid grid-cols-1 gap-6',
        // SLOT: flyer plate. The second column EXISTS ONLY WHEN THERE IS A
        // FLYER. A show with no image collapses to one full-width column
        // rather than reserving a gutter for a placeholder. The earlier
        // always-on plate promised an image that was never coming, and every
        // flyerless show paid a column of whitespace for it.
        //
        // EXACTLY TWO CHILDREN by construction: the content column and the
        // plate. A third direct child would be auto-placed into row 2 of the
        // narrow first track, under the flyer. New modules go INSIDE the
        // content div below, not beside it.
        flyerSrc && 'md:grid-cols-[minmax(0,18rem)_minmax(0,1fr)] md:gap-8'
      )}
    >
      <div className="min-w-0">
        {/* SLOT: header block. Date, bill, sold-out flag. */}
        <div className="flex items-center gap-2 mb-2">
          <span className="text-lg font-bold text-primary">
            {formatShowDate(show.event_date, timing.state, false, timing.timezone)}
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

        {/* The bill, typeset: who is playing, who they record for, where they
            are from. Labels and hometown sit INLINE with the name rather than
            on their own meta row, so one act reads as one line of type.

            They are inside the h1 on purpose. The heading is the show's title
            and "Modest Mouse [Epic] Issaquah, WA" is that title as the mock
            states it; splitting the annotations out to a sibling would make
            the visual line and the document's heading disagree. The brackets
            and separators are aria-hidden, so the announced name is
            "Modest Mouse Epic Issaquah, WA". */}
        {/* `break-words`: a label or band name can be one long unbroken token
            (a URL-ish name, a 200-character joke name), and in an 18rem-wide
            reading column an unbreakable token would push the whole line past
            the viewport. */}
        <h1 className="text-2xl md:text-3xl font-bold leading-8 md:leading-9 break-words">
          {effectiveHeadliners.map((artist, index) => (
            <span key={artist.id}>
              {/* Same rule as BillLabels' middot: the glyph is decoration and
                  is hidden, the spaces around it are real text and stay in the
                  accessibility tree, so a co-headline bill is announced as
                  "Modest Mouse ... Califone ..." rather than as a bullet. */}
              {index > 0 && (
                <>
                  {' '}
                  <span
                    aria-hidden="true"
                    className="text-muted-foreground/60 font-normal"
                  >
                    &bull;
                  </span>{' '}
                </>
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
              <BillLabels labels={artist.labels} className="text-base" />
              <BillHometown artist={artist} className="text-base" />
            </span>
          ))}
        </h1>
        {effectiveSupport.length > 0 && (
          // One act per line, hanging under a single `w/`, rather than one
          // comma-run. With hometowns on the line a run reads as garbage:
          // "Califone [Dead Oceans] Chicago, IL, Other Band [Merge] Austin,
          // TX" puts the same comma between a state and the next band. The
          // empty grid cell on lines 2+ is what keeps them aligned under the
          // first name without repeating the marker as text an assistive
          // reader would hear on every line.
          <div className="mt-1 grid grid-cols-[auto_minmax(0,1fr)] gap-x-2 gap-y-0.5 text-lg text-muted-foreground break-words">
            {effectiveSupport.map((artist, index) => (
              <Fragment key={artist.id}>
                {index === 0 ? (
                  // The trailing space is a real character: the marker and the
                  // name are separate grid cells, so the gap between them is
                  // layout, not text, and a reader that flattens the line would
                  // otherwise get "w/Califone".
                  <span className="italic">w/ </span>
                ) : (
                  // Empty, so it is already absent from the accessibility
                  // tree; it exists only to fill the marker column.
                  <span />
                )}
                <div>
                  {artist.slug ? (
                    <Link
                      href={`/artists/${artist.slug}`}
                      className="font-medium text-foreground hover:text-primary transition-colors"
                    >
                      {artist.name}
                    </Link>
                  ) : (
                    <span className="font-medium text-foreground">
                      {artist.name}
                    </span>
                  )}
                  {/* Both annotations at one size: they are the same class of
                      fact and sit on the same line under an 18px name. */}
                  <BillLabels labels={artist.labels} className="text-sm" />
                  <BillHometown artist={artist} className="text-sm" />
                  {/* LAST, after the hometown. The mock's locked sequence is
                      name, labels, hometown with nothing interposed, and this
                      annotation predates the mock (see
                      SUPPORT_SET_TYPE_LABELS). Keeping it but putting it at
                      the end of the line is what lets both be true. */}
                  <SupportSetTypeLabel setType={artist.set_type} />
                </div>
              </Fragment>
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
            {/* Testid rather than a text query: the bill above now prints each
                act's hometown, so "Phoenix, AZ" is no longer a unique string on
                this page. */}
            <div
              data-testid="venue-location"
              className="flex items-center gap-1 text-muted-foreground mt-1"
            >
              <MapPin className="h-4 w-4" />
              {/* Same locked rule as the bill above, through the same helper.
                  The hand-written template this replaced printed a trailing
                  comma for a venue with no state. */}
              <span>
                {formatLocation({ city: venue.city, state: venue.state })}
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
            right-hand column of the header and was moved, not rewired.

            Two components in here share state: `ShowAddToCalendar` in the meta
            row below saves the show as a side effect and dedupes against the
            `SaveButton` inside `actions` through the shared query key. They now
            render three rows apart. Move either one and check the other. */}
        <div className="mt-4 border-t border-border/60 pt-4">
          {/* The calendar affordance sits here with the when-info (event-page
              convention), not in the social action cluster. */}
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
            <span>{formatShowTime(show.event_date, timing.state, timing.timezone)}</span>
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
            here, under the actions and above the description. Deliberately
            empty: the counts are designed but not built, and reserving visible
            blank space for them would read as a broken module. Nothing enforces
            this position, so treat it as the intent it is, not a guarantee. */}

        {/* Description */}
        {show.description && (
          <p className="mt-4 text-muted-foreground">{show.description}</p>
        )}
      </div>

      {/* The plate is LAST in the document and first in the desktop grid.
          Reading order is the reason, and it goes both ways. On a phone the
          columns stack, and a full-width flyer at the top pushed the date, the
          bill and the venue clean off the first screen, which is the opposite
          of what somebody checking a show on their phone came for; here the
          typeset facts come first and the poster follows them. For a screen
          reader, at every width, the bill is the content and the flyer is the
          supplement, which is the order they are now announced in.

          `md:order-first` puts it back in the left column at desktop, where
          the mock has it. */}
      {flyerSrc && (
        <ShowFlyerPlate
          // A new URL gets a NEW element rather than a reused one. The plate
          // reads `complete` / `naturalWidth` off the node on mount, and on a
          // reused element those still describe the PREVIOUS image at that
          // moment (the spec queues the image update as a microtask), so a
          // good replacement could be judged on the old one's failure.
          key={flyerSrc}
          src={flyerSrc}
          credit={flyerCredit(show)}
          onError={handleFlyerError}
          className="md:order-first"
        />
      )}
    </div>
  )
}
