import { playableMusicSources } from '@/lib/playableMusicSources'
import { byBillPosition, splitBill } from '../utils'
import type { ArtistResponse } from '../types'

/**
 * Where the card's audio comes from, as the reader is told it.
 *
 * A display string rather than a code, because it is rendered verbatim in the
 * card's meta line (`Califone · Bandcamp · [Buy] [Share]`) and there is no
 * second consumer that would want it in another form.
 */
export type ListenSource = 'Bandcamp' | 'Spotify'

export interface ListenCard {
  artist: ArtistResponse
  source: ListenSource
  /**
   * The Bandcamp release page: what `[Buy]` points at, and what the player is
   * built from. Null exactly when `source` is `Spotify`, which sells nothing
   * and gets no bracket rather than an invented one.
   *
   * Either `artist.bandcamp_embed_url` or null, never a value derived from
   * something else, so this is that field under a name that says what it is
   * FOR. Read it rather than reaching back to the column at a render site: the
   * column is untrusted and this is the checked copy (see `toListenCard`).
   */
  buyHref: string | null
}

/**
 * One card per bill artist who has a PLAYER, in the order the bill reads.
 *
 * "A player" is the whole gate, and it is stricter than "has a music link".
 * A card here is a stack-mate of two other players and looks exactly like
 * them, so a card whose body turns out to be a text link out to Bandcamp is
 * a card that lied about itself. Only an embeddable release URL and a
 * parseable Spotify URL qualify; a bare Bandcamp PROFILE does not, and neither
 * does an unparseable Spotify URL. The show page's old gate
 * (`bandcamp_embed_url || socials.spotify || socials.bandcamp`) accepted all
 * three, which is why it could render a heading over silence.
 *
 * Ordering is the two-step the bill block itself uses: sort by
 * `show_artists.position`, THEN hoist whoever is curated as a headliner. Both
 * halves are needed, because `set_type` is authoritative at ANY position, so a
 * bill submitted in stage order has its headliner at the bottom. Sorting alone
 * would print the header's running order backwards a few hundred pixels below
 * the header.
 *
 * The source ladder is `playableMusicSources` (`lib/playableMusicSources`), the
 * same one `MusicEmbed` walks, so a card and the player inside it cannot
 * disagree about which of an artist's URLs is the one to use. It has to agree,
 * because `MusicEmbed` renders NOTHING when it can find no source.
 *
 * This surface asks that ladder for the `'release'` scope, the stricter of the
 * two: a card is a stack-mate of two other players and a `[Buy]` promises ONE
 * record. So `https://band.bandcamp.com/music` gets no card here while
 * `hasRenderableMusic` (`lib/musicAvailability`) still opens a music block for
 * it elsewhere: correct, not drift.
 *
 * What no synchronous predicate can settle: a stored release URL that does not
 * resolve to a player id. `MusicEmbed` then falls through to the artist's
 * Spotify embed if it has one (the card plays, under a label that still says
 * "Bandcamp"), and otherwise to its own outbound link to that same release page
 * (the card does not play, and shows a link one line under the `[Buy]` bracket
 * pointing at the same place).
 *
 * Do not read that as an outage-only state. The likelier cause is a STEADY one:
 * stored embed URLs are never revalidated, so a release the band renamed,
 * un-published, or deleted answers 404 forever, and the route reports "no
 * embeddable player" for a page that loads fine but no longer carries the
 * descriptor. This gate cannot see any of that, because the answer only exists
 * over the network. What it CAN keep out is the card that was never going to
 * have a player, and that is the line it draws.
 */
export function listenCardsForBill(artists: ArtistResponse[]): ListenCard[] {
  const { headliners, support } = splitBill([...artists].sort(byBillPosition))
  return [...headliners, ...support]
    .map(toListenCard)
    .filter((card): card is ListenCard => card !== null)
}

function toListenCard(artist: ArtistResponse): ListenCard | null {
  // The top rung only. A card carries one player, so the alternatives below the
  // winner are of no use to it.
  //
  // `bandcamp_embed_url` is contributor-writable, so the `'release'` scope is
  // what proves the value before any of it is shown. An unproven value would
  // reach three places at once: the word "Bandcamp" in the meta line, a `[Buy]`
  // bracket announced as "on Bandcamp", and (once the resolve fails)
  // `MusicEmbed`'s own outbound fallback link, which is the card's entire body.
  // Taking the claim and the href from the same proven rung is what keeps them
  // one decision, and it lets an artist with a junk Bandcamp value still get
  // their working Spotify player.
  const [source] = playableMusicSources({
    bandcampUrl: artist.bandcamp_embed_url,
    spotifyUrl: artist.socials?.spotify,
    bandcampScope: 'release',
  })

  // A bare Bandcamp PROFILE reaches no rung at this scope and so gets no card.
  // `MusicEmbed` would render an outbound text link for it, not a player, and a
  // link wearing the same border as the two players above it is a card that
  // misrepresents itself. The artist page still carries the profile link.
  if (!source) {
    return null
  }

  // No `[Buy]` on the Spotify rung: Spotify does not sell the record, and
  // pointing the word somewhere else would invent an affordance.
  return source.service === 'bandcamp'
    ? { artist, source: 'Bandcamp', buyHref: source.url }
    : { artist, source: 'Spotify', buyHref: null }
}
