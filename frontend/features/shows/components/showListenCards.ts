import { parseSpotifyEmbed } from '@/lib/spotify'
import { byBillPosition } from '../utils'
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
   * Outbound purchase target for `[Buy]`, or null when there is nothing to
   * buy. Only a Bandcamp album/track page sells a record; a Spotify embed and
   * a bare Bandcamp profile both get no bracket rather than an invented one.
   *
   * Always either `artist.bandcamp_embed_url` or null, so this is that field
   * under a name that says what it is FOR, not a second source of truth. Read
   * it rather than re-deriving "is there something to buy" at a render site,
   * which is where the two would drift.
   */
  buyHref: string | null
}

/**
 * One card per bill artist who actually has something to play, in bill order.
 *
 * The ladder below MIRRORS `MusicEmbed`'s own `deriveEmbedState` (bandcamp
 * album/track → spotify → bandcamp fallback link), and that duplication is a
 * DEBT, not a design. It has to agree, because `MusicEmbed` renders NOTHING
 * when it can find no source and a looser gate here would hand the reader an
 * empty bordered card with a meta line and silence under it. The show page's
 * old gate (`bandcamp_embed_url || socials.spotify || socials.bandcamp`) was
 * exactly that looser gate: an artist whose only music link is an unparseable
 * Spotify URL passes it and renders nothing.
 *
 * Three other surfaces hand-mirror the same ladder for the same reason
 * (`ArtistPanel`, `ArtistContextPanel`, and `ShowCard`, the last still on the
 * loose version). The unification they all want is one exported pure resolver
 * beside `deriveEmbedState` that every gate calls; it is owed, and it is a
 * wider change than this surface. Do not add a fifth copy.
 *
 * `parseSpotifyEmbed` is therefore load-bearing, not decorative — it is the
 * same host-anchored validation `MusicEmbed` runs, so the two agree on which
 * Spotify URLs are real.
 *
 * The one case the two can still disagree on: a stored `bandcamp_embed_url`
 * whose id resolve FAILS at request time falls through to the artist's Spotify
 * embed inside `MusicEmbed`, while this label still says "Bandcamp". The resolve
 * happens over the network after render, so no synchronous predicate can know.
 * The `[Buy]` href stays correct either way (it points at the Bandcamp page we
 * were given), so the mismatch is a stale source WORD during a Bandcamp outage,
 * not a wrong link.
 */
export function listenCardsForBill(artists: ArtistResponse[]): ListenCard[] {
  return [...artists]
    .sort(byBillPosition)
    .map(toListenCard)
    .filter((card): card is ListenCard => card !== null)
}

function toListenCard(artist: ArtistResponse): ListenCard | null {
  // Priority 1: an album/track page. The only unit that is both embeddable and
  // buyable, so it is also the only one that earns a `[Buy]`.
  if (artist.bandcamp_embed_url) {
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

  // Priority 3: a bare Bandcamp profile. `MusicEmbed` renders its own visible
  // "Listen to X on Bandcamp" link for this case, so the card carries no
  // `[Buy]` bracket that would say the same thing twice a line apart.
  if (artist.socials?.bandcamp) {
    return { artist, source: 'Bandcamp', buyHref: null }
  }

  return null
}
