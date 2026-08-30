import { isBandcampReleaseUrl } from '@/lib/bandcamp'
import { parseSpotifyEmbed } from '@/lib/spotify'
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
 * The source ladder MIRRORS `MusicEmbed`'s own `deriveEmbedState`, and that
 * duplication is a DEBT, not a design. It has to agree, because `MusicEmbed`
 * renders NOTHING when it can find no source. Three other surfaces hand-mirror
 * the same ladder for the same reason (`ArtistPanel`, `ArtistContextPanel`,
 * and `ShowCard`, the last still on the loose version). The unification they
 * all want is one exported pure resolver beside `deriveEmbedState` that every
 * gate calls; it is owed, and it is a wider change than this surface. Do not
 * add a fifth copy.
 *
 * `parseSpotifyEmbed` and `isBandcampReleaseUrl` are therefore load-bearing,
 * not decorative: they are the same validation the player and the resolver
 * route run, so the three agree on which URLs are real.
 *
 * What no synchronous predicate can settle: a stored release URL whose id
 * resolve FAILS at request time. `MusicEmbed` then falls through to the
 * artist's Spotify embed if it has one (the card plays, under a label that
 * still says "Bandcamp"), and otherwise to its own outbound link to that same
 * release page (the card does not play, and shows the reader a link one line
 * under the `[Buy]` bracket pointing at the same place). Both are degraded
 * states of a real player during a Bandcamp outage rather than a card that
 * never had one, which is the line this gate draws.
 */
export function listenCardsForBill(artists: ArtistResponse[]): ListenCard[] {
  const { headliners, support } = splitBill([...artists].sort(byBillPosition))
  return [...headliners, ...support]
    .map(toListenCard)
    .filter((card): card is ListenCard => card !== null)
}

function toListenCard(artist: ArtistResponse): ListenCard | null {
  // Priority 1: a Bandcamp release page. The only unit that is both embeddable
  // and buyable, so it is also the only one that earns a `[Buy]`.
  //
  // The URL has to PROVE it is one, and the whole branch is gated on that
  // rather than just the bracket. `bandcamp_embed_url` is contributor-writable
  // and is not URL-checked on write: it sits in the artist edit allowlist with
  // no entry in the backend's URL field specs, a trusted-tier edit
  // self-approves, and the entity-request path validates it as any http(s) URL
  // by its own admission. So the column can hold an arbitrary host.
  //
  // An ungated branch would put that host in three places at once: the word
  // "Bandcamp" in the meta line, a `[Buy]` bracket announced as "on Bandcamp",
  // and (once the resolve fails) `MusicEmbed`'s own outbound fallback link,
  // which is the card's entire body. That is the phishing shape. Gating here
  // instead of at each render site is what makes the claim and the href one
  // decision, and it lets an artist with a junk Bandcamp value still get their
  // working Spotify player below.
  if (
    artist.bandcamp_embed_url &&
    isBandcampReleaseUrl(artist.bandcamp_embed_url)
  ) {
    return {
      artist,
      source: 'Bandcamp',
      buyHref: artist.bandcamp_embed_url,
    }
  }

  // Priority 2: a Spotify artist/album/track URL that survives host-anchored
  // parsing. No `[Buy]` — Spotify does not sell the record, and pointing the
  // word somewhere else would be inventing an affordance the mock did not ask
  // for.
  if (artist.socials?.spotify && parseSpotifyEmbed(artist.socials.spotify)) {
    return { artist, source: 'Spotify', buyHref: null }
  }

  // A bare Bandcamp PROFILE deliberately gets nothing. `MusicEmbed` would
  // render an outbound text link for it, not a player, and a link wearing the
  // same border as the two players above it is a card that misrepresents
  // itself. The artist page still carries the profile link.
  return null
}
