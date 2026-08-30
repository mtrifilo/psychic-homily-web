'use client'

import Link from 'next/link'
import {
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
 * with something to play, headliner first.
 *
 * Players load OPEN (locked decision 9). There is no facade, no click-to-load,
 * and no single-top-track reduction: the reader is scanning six or seven
 * unheard bills a night, and a card that costs a click before it can cost a
 * listen is a card they skip. The iframes carry `loading="lazy"` inside
 * `MusicEmbed`, which is a fetch-timing detail the reader never sees — a
 * card on screen is already playable.
 *
 * The mock draws each card as artwork + release title + a transport row with a
 * scrubber and a duration. Those four things all live INSIDE the third-party
 * player: they are what the Bandcamp iframe renders, and it exposes no API for
 * a host page to draw its own transport over. What this module owns is the
 * chrome around the player — the section label, the card, and the meta line
 * with its outbound verbs.
 */
export function ShowListenModule({ artists }: ShowListenModuleProps) {
  const cards = listenCardsForBill(artists)

  // No cards, no header. `listenCardsForBill` mirrors MusicEmbed's own
  // resolution contract precisely so this can be trusted — see its docblock.
  if (cards.length === 0) return null

  return (
    <section className="mb-8" data-testid="show-listen-module">
      <SectionHeader title="Listen / Before you go" as="h2" size="md" />
      {/* A real list: the count is the useful thing a screen reader can say
          about this module before deciding whether to walk it.

          The width cap is a deliberate deviation from the mock, which draws
          full-bleed cards. `MusicEmbed` caps the Bandcamp iframe at 700px
          because that player's internal layout is fixed and does not stretch,
          so a full-width card would frame it with a 400px dead zone at 1280.
          Capping the LIST instead of reaching into the shared primitive keeps
          every card the same width — which is what the mock is actually
          saying — and leaves the section label spanning the column. */}
      <ul className="mt-3 max-w-[700px] space-y-2">
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
  const { artist, source } = card
  const verbs = listenVerbs(card)

  const segments = [
    artist.slug ? (
      <Link
        key="artist"
        href={`/artists/${artist.slug}`}
        className="text-foreground transition-colors hover:text-primary"
      >
        {artist.name}
      </Link>
    ) : (
      // `artists.slug` is nullable and the backend flattens null to "", so an
      // empty slug must render as text: `/artists/` resolves to the INDEX, not
      // a 404, and would quietly send the reader to the wrong page.
      <span key="artist">{artist.name}</span>
    ),
    source,
    ...(verbs ? [verbs] : []),
  ]

  return (
    <div className="rounded-sm border border-border/60 px-3 pt-2.5 pb-0.5">
      <MiddotSegments
        segments={segments}
        keys={['artist', 'source', 'verbs']}
        className="mb-1.5 font-mono text-xs text-muted-foreground"
        data-testid="listen-card-meta"
      />
      <MusicEmbed
        bandcampAlbumUrl={artist.bandcamp_embed_url}
        bandcampProfileUrl={artist.socials?.bandcamp}
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
    <span key="verbs" className="inline-flex items-baseline gap-x-2">
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
