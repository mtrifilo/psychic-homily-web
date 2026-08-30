'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import {
  BANDCAMP_EMBED_MAX_WIDTH_PX,
  BracketLink,
  MusicEmbed,
  SectionHeader,
  ShareButton,
  SocialLinks,
} from '@/components/shared'
import { MiddotSegments } from './MiddotSegments'
import { listenCardsForBill, type ListenCard } from './showListenCards'
import { billHometown } from '../utils'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ArtistResponse } from '../types'

interface ShowListenModuleProps {
  /** The show's bill, in any order — the module sorts by bill position. */
  artists: ArtistResponse[]
  /**
   * Server-computed. Selects the section HEADING's register only; the cards
   * and their players are identical in every state.
   */
  lifecycle: ShowLifecycleState
}

/**
 * The section heading: `Listen / Before you go` while the show is ahead of the
 * reader, bare `Listen` once it is not.
 *
 * "Before you go" is an instruction to a reader who can still go, so it cannot
 * survive the show. Dropping the qualifier is a subtraction rather than new
 * copy, and it is as far as this module can honestly move: the PAST mock heads
 * this section `WHAT / THEY PLAYED`, which describes a setlist. These cards are
 * the bill's RELEASES — what each act has recorded, not what they performed —
 * so that heading needs the show-to-recording link the schema does not carry.
 */
function listenModuleTitle(lifecycle: ShowLifecycleState): string {
  return lifecycle === 'past' ? 'Listen' : 'Listen / Before you go'
}

/**
 * The mock's `LISTEN / BEFORE YOU GO` module: one dense card per bill artist
 * with a player, headliner first.
 *
 * Players load OPEN (locked decision 9). There is no facade, no click-to-load,
 * and no single-top-track reduction: the reader is scanning six or seven
 * unheard bills a night, and a card that costs a click before it can cost a
 * listen is a card they skip. The iframes carry `loading="lazy"` inside
 * `MusicEmbed`, which is a fetch-timing detail the reader never sees.
 *
 * Every card is built from a source that CAN carry a player, which is not the
 * same promise as every card showing one. A stored Bandcamp release URL that no
 * longer resolves (the band renamed or pulled the record, or Bandcamp is
 * throttling) leaves an act with no Spotify holding `MusicEmbed`'s own outbound
 * link to that release instead of the iframe. Only the network knows, and it
 * knows after render. What `listenCardsForBill` can rule out, and does, is the
 * card that was never going to have a player at all, which is why a bare
 * Bandcamp profile earns no card.
 *
 * The mock draws each card as artwork + release title + a transport row with a
 * scrubber and a duration. Those four things all live INSIDE the third-party
 * player: they are what the Bandcamp iframe renders, and it exposes no API for
 * a host page to draw its own transport over. What this module owns is the
 * chrome around the player: the section label, the card, the meta line with its
 * outbound verbs, and the act's hometown and social links.
 *
 * Those last two are a deliberate departure from the mock, which draws neither.
 * The owner ruled on 2026-08-30 that they stay: the hometown is what tells a
 * reader whether the unheard band is a local one, and the socials are the act's
 * own destinations, which is a different thing from the page's player. See
 * {@link ShowListenCard} for where they sit and what that costs in height.
 */
export function ShowListenModule({
  artists,
  lifecycle,
}: ShowListenModuleProps) {
  // Memoized for prop identity rather than for the arithmetic: the cards are
  // fresh objects on every ShowDetail re-render (an edit toggle, a save banner,
  // an admin status mutation), and a card that changes identity every render
  // cannot be handed to a memoized child later. Same reason ShowCard memoizes
  // its `splitBill` call.
  const cards = useMemo(() => listenCardsForBill(artists), [artists])

  // No cards, no header. `listenCardsForBill` mirrors MusicEmbed's own
  // resolution contract precisely so this can be trusted — see its docblock.
  if (cards.length === 0) return null

  return (
    <section className="mb-8" data-testid="show-listen-module">
      <SectionHeader
        title={listenModuleTitle(lifecycle)}
        as="h2"
        size="md"
      />
      {/* A real list: the count is the useful thing a screen reader can say
          about this module before deciding whether to walk it.

          The width cap is a deliberate deviation from the mock, which draws
          full-bleed cards. It is a property of the PLAYERS: the Bandcamp embed
          has a fixed internal layout and stops at
          `BANDCAMP_EMBED_MAX_WIDTH_PX`, so a full-bleed card would frame it
          with a 400px dead zone at 1280, while the Spotify embed has no cap of
          its own and would stretch to a different width in the card beside it.
          Capping the LIST is what makes every card the same width, which is
          what the mock is really saying, and leaves the section label spanning
          the column. */}
      <ul
        className="mt-3 space-y-2"
        style={{ maxWidth: BANDCAMP_EMBED_MAX_WIDTH_PX }}
      >
        {cards.map(card => (
          <li key={card.artist.id}>
            <ShowListenCard card={card} />
          </li>
        ))}
      </ul>
    </section>
  )
}

/**
 * One card: the meta line and the act's socials, then the player.
 *
 * Padding is deliberately asymmetric. `MusicEmbed` owns a `mb-2` on its own
 * wrapper (it is a section in its own right elsewhere), so a symmetric `py`
 * here would read as a card with a fat bottom gutter. The bottom padding is
 * shrunk by that same 8px instead of reaching into the shared primitive for a
 * className it does not take.
 *
 * The hometown is a SEGMENT of the meta line and the socials share that line's
 * row rather than either taking a row of its own: both restore an affordance
 * the mock omits (owner decision, 2026-08-30), and the mock's whole subject is
 * density, so they are placed where they cost the fewest pixels. The socials
 * are the taller of the two at 36px buttons, which sets the row height; the
 * hometown costs nothing. Below the card's own width the row wraps and the
 * icons drop under the meta line rather than crushing it.
 */
function ShowListenCard({ card }: { card: ListenCard }) {
  const { artist, source, buyHref } = card
  const hometown = billHometown(artist)

  // Segment and key pushed together, the way ShowProvenanceLine builds its
  // byline: `MiddotSegments` reads the two as parallel arrays, so building them
  // apart means a segment added in the middle mis-keys everything after it with
  // nothing to catch the slip.
  const segments: React.ReactNode[] = []
  const keys: string[] = []
  const push = (key: string, segment: React.ReactNode) => {
    segments.push(segment)
    keys.push(key)
  }

  push(
    'artist',
    artist.slug ? (
      <Link
        href={`/artists/${artist.slug}`}
        className="text-foreground transition-colors hover:text-primary"
      >
        {artist.name}
      </Link>
    ) : (
      // `artists.slug` is nullable and the backend flattens null to "", so an
      // empty slug must render as text: `/artists/` resolves to the INDEX, not
      // a 404, and would quietly send the reader to the wrong page.
      <span>{artist.name}</span>
    )
  )
  if (hometown) {
    push(
      'hometown',
      // Read aloud, a city is one more proper noun in a row of them unless
      // something says "from". Same connective, and the same
      // space-outside-the-hidden-span placement, as the bill block's hometown.
      <span>
        <span className="sr-only">from</span> {hometown}
      </span>
    )
  }
  push('source', source)
  const verbs = listenVerbs(card)
  if (verbs) push('verbs', verbs)

  return (
    <div className="rounded-sm border border-border/60 px-3 pt-2.5 pb-0.5">
      <div className="mb-1.5 flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5">
        <MiddotSegments
          segments={segments}
          keys={keys}
          className="min-w-0 font-mono text-xs text-muted-foreground"
          // Per CARD, not per page: reach it through `within(card)`, never a
          // bare `getByTestId`, which throws on any bill with two playable
          // acts.
          data-testid={`listen-card-meta-${artist.id}`}
        />
        {/* Renders nothing for an act with no social links, so the row
            collapses back to the meta line alone. The redundancy with the
            player's own source is accepted: these are the act's OWN
            destinations, restored per owner decision, and the player is the
            page's. */}
        <SocialLinks social={artist.socials} className="shrink-0" />
      </div>
      <MusicEmbed
        // The CHECKED copy of the column, not the column. `listenCardsForBill`
        // has already established that this is a real Bandcamp release page;
        // handing `MusicEmbed` the raw field would put an unvalidated host into
        // its outbound fallback link on the resolve-failure path.
        bandcampAlbumUrl={buyHref}
        // No `bandcampProfileUrl`: the gate emits no card that could reach
        // MusicEmbed's profile branch, so passing one would be a prop with no
        // reachable behaviour and no way to test it.
        spotifyUrl={artist.socials?.spotify}
        artistName={artist.name}
        // Suppresses MusicEmbed's own "Music" heading, which on this page used
        // to nest one h2 per artist under the section's h2. The card's meta
        // line is the label now.
        compact
      />
    </div>
  )
}

/**
 * The `[Buy] [Share]` cluster, or null when neither bracket can render.
 *
 * Returned as one unit because `MiddotSegments` requires its callers to filter
 * absences BEFORE handing over the list — a segment that renders empty leaves a
 * dangling separator. Share is gated on the slug for the same reason
 * `ShareButton` documents: `/artists/${slug}` with an empty slug is a path no
 * guard downstream can tell apart from a real one.
 *
 * One residual case this cannot pre-filter: `ShareButton` also renders nothing
 * where the browser exposes neither the Web Share API nor a clipboard (an
 * insecure origin — a phone on a plain-http dev server). A Spotify card with a
 * slug on such an origin shows a trailing middot. Accepted rather than
 * mirrored, because mirroring would mean duplicating a capability probe that
 * only that browser can answer.
 */
function listenVerbs({ artist, source, buyHref }: ListenCard) {
  const sharePath = artist.slug ? `/artists/${artist.slug}` : null
  if (!buyHref && !sharePath) return null

  return (
    <span className="inline-flex items-baseline gap-x-2">
      {buyHref && (
        // Capitalized against the mock's lowercase `[buy]`: `ShareButton` owns
        // its own label and capitalizes it, and two cases in one bracket pair
        // reads as a typo. No `↗` — this is a dense meta line, and BracketLink
        // appends the new-tab announcement itself.
        <BracketLink
          label="Buy"
          href={buyHref}
          external
          ariaLabel={`Buy ${artist.name} on ${source}`}
          // BracketLink's own floor is 14px; this line is the 12px mono meta
          // line, and one oversized bracket mid-line reads as a mistake. Same
          // adjustment ShowTicketRow makes in the other direction.
          className="text-xs"
        />
      )}
      <ShareButton
        path={sharePath}
        variant="bracket"
        ariaLabel={`Share ${artist.name}`}
        className="text-xs"
      />
    </span>
  )
}
