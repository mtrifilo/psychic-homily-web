'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import {
  BANDCAMP_EMBED_MAX_WIDTH_PX,
  BracketLink,
  MusicEmbed,
  SectionHeader,
  ShareButton,
} from '@/components/shared'
import { MiddotSegments } from './MiddotSegments'
import { listenCardsForBill, type ListenCard } from './showListenCards'
import type { ArtistResponse } from '../types'

interface ShowListenModuleProps {
  /** The show's bill, in any order — the module sorts by bill position. */
  artists: ArtistResponse[]
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
 * Every card mounts a real player. The one state where a card can be on screen
 * without one is a Bandcamp id resolve that fails at request time for an act
 * with no Spotify to fall back on; `MusicEmbed` then shows its own link to the
 * same release page. That is a Bandcamp outage degrading a player, not a card
 * that never had one. `listenCardsForBill` is what keeps the second case from
 * existing, and it is the reason a bare Bandcamp profile earns no card.
 *
 * The mock draws each card as artwork + release title + a transport row with a
 * scrubber and a duration. Those four things all live INSIDE the third-party
 * player: they are what the Bandcamp iframe renders, and it exposes no API for
 * a host page to draw its own transport over. What this module owns is the
 * chrome around the player — the section label, the card, and the meta line
 * with its outbound verbs.
 */
export function ShowListenModule({ artists }: ShowListenModuleProps) {
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
      <SectionHeader title="Listen / Before you go" as="h2" size="md" />
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
 * One card: the meta line, then the player.
 *
 * Padding is deliberately asymmetric. `MusicEmbed` owns a `mb-2` on its own
 * wrapper (it is a section in its own right elsewhere), so a symmetric `py`
 * here would read as a card with a fat bottom gutter. The bottom padding is
 * shrunk by that same 8px instead of reaching into the shared primitive for a
 * className it does not take.
 */
function ShowListenCard({ card }: { card: ListenCard }) {
  const { artist, source, buyHref } = card

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
  push('source', source)
  const verbs = listenVerbs(card)
  if (verbs) push('verbs', verbs)

  return (
    <div className="rounded-sm border border-border/60 px-3 pt-2.5 pb-0.5">
      <MiddotSegments
        segments={segments}
        keys={keys}
        className="mb-1.5 font-mono text-xs text-muted-foreground"
        // Per CARD, not per page: reach it through `within(card)`, never a bare
        // `getByTestId`, which throws on any bill with two playable acts.
        data-testid={`listen-card-meta-${artist.id}`}
      />
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
