'use client'

import { Fragment, useCallback, useState } from 'react'
import Link from 'next/link'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { formatLocation } from '@/lib/formatLocation'
import { formatShowDate } from '@/lib/utils/formatters'
import { ShowFlyerPlate } from './ShowFlyerPlate'
import { ShowTicketRow } from './ShowTicketRow'
import { ShowVenueModule } from './ShowVenueModule'
import { flyerImageSrc, sourceVenueName } from './showFlyer'
import { showTimingInput, splitBill } from '../utils'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
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
   * Server-computed lifecycle, threaded to the ticket row so the sale-state
   * claim can respect the archive (ON SALE is present tense). Same value the
   * status stripe above this header renders from.
   */
  lifecycle: ShowLifecycleState
  /**
   * The ADMIN/OWNER moderation cluster (edit, delete, status flags),
   * rendered at the foot of the ticket block at every width. The caller
   * gates it — pass `undefined` for public viewers so the slot reserves no
   * margin. The public verbs (buy, calendar, save, collect, share) live
   * inside {@link ShowTicketRow}, not here.
   */
  actions?: React.ReactNode
}

/**
 * ShowDetail-specific header block. Owns the bill-position artist rendering
 * (headliners as h1, support as "w/ ..." row) and the date + sold-out badge
 * row directly; composes the venue block from {@link ShowVenueModule}
 * (name + address, facts line, venue verbs) and the how-much line + public
 * action row from {@link ShowTicketRow}; ends with the description
 * paragraph.
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
export function ShowHeader({ show, lifecycle, actions }: ShowHeaderProps) {
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
  // Memoised as ordinary prop hygiene, not a correctness requirement: the
  // plate's pre-attach ref holds this in a latest-ref, so its identity no
  // longer affects when that ref attaches.
  const handleFlyerError = useCallback(
    () => setFailedFlyerSrc(candidateFlyerSrc),
    [candidateFlyerSrc]
  )

  // The same zone the status stripe above this block is judged on. They render
  // a few hundred pixels apart, so a page that resolved the venue's calendar
  // two ways could print two different dates in one screenshot.
  const timing = showTimingInput(show)
  // Sort the whole bill first so every downstream slice — including the
  // `artists[0]` / `artists.slice(1)` fallback below — is position-ordered.
  const artists = [...show.artists].sort(byBillPosition)

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

        {/* SLOT: venue module. The co-primary entity's block, ABOVE the
            ticket module (locked scan order: who → where/when → how much →
            social). Name + address, facts line, venue verbs — see
            ShowVenueModule. Self-hides for a venue-less show. */}
        <ShowVenueModule show={show} />

        {/* SLOT: ticket and action block. The what-it-costs line and every
            verb a reader can act on, together in one band under the venue, as
            the mock has them (PSY-1686). The remaining `actions` cluster is
            admin/owner chrome (edit, delete, status flags) — the public verbs
            live inside ShowTicketRow's bracket row. */}
        <div className="mt-4 border-t border-border/60 pt-4">
          <ShowTicketRow show={show} lifecycle={lifecycle} />

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
          // Deliberately NOT keyed on the URL, matching CollectionCoverImage.
          // The plate's pre-attach ref is identity-stable, so it cannot re-read
          // a reused node whose state still describes the previous image, and
          // a later failure is caught by `onError`, which React attaches at
          // element creation. Keying would only cost the browser's seamless
          // swap: it keeps painting the current flyer until the replacement
          // has fully loaded, instead of blanking the column for the length of
          // a third-party fetch every time an admin fixes an `image_url`.
          src={flyerSrc}
          credit={sourceVenueName(show)}
          onError={handleFlyerError}
          className="md:order-first"
        />
      )}
    </div>
  )
}
