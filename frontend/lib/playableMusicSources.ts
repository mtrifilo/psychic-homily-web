import { isAllowedBandcampUrl, isBandcampReleaseUrl } from '@/lib/bandcamp'
import { parseSpotifyEmbed, type SpotifyEmbedKind } from '@/lib/spotify'

/**
 * How much a stored Bandcamp URL has to prove before this ladder offers it.
 *
 * `'release'` admits only a page naming ONE release (`isBandcampReleaseUrl`).
 * That is the scope a surface needs when it will also NAME the source to the
 * reader or point a Buy bracket at it, because both claims are about one
 * record.
 *
 * `'platform'` admits any page on bandcamp.com (`isAllowedBandcampUrl`). It is
 * enough for a caller that will hand the URL to `/api/bandcamp/album-id`, which
 * reads the page and answers with a player id or nothing: a profile or merch
 * page can still carry a player, so the stricter rule would hide players that
 * do render.
 *
 * The two scopes are the ONLY axis on which the callers differ.
 */
export type BandcampScope = 'release' | 'platform'

/**
 * A stored URL that has proven it can supply a player.
 *
 * `url` is the input string, unchanged, and it has cleared the predicate named
 * by the scope, so a caller may render it as an href without re-testing it. The
 * Spotify arm additionally carries the parsed embed target, because the iframe
 * src is built from `kind` + `id` and never from the raw input.
 */
export type PlayableMusicSource =
  | { service: 'bandcamp'; url: string }
  | { service: 'spotify'; url: string; kind: SpotifyEmbedKind; id: string }

/**
 * Every source that could play this artist, in the order a surface must try
 * them.
 *
 * Bandcamp outranks Spotify: it is the artist's own store, so it is the only
 * rung that can also carry a Buy, and a surface that found one has no reason to
 * look further. The ordering and both admission predicates live here so the
 * player component and the show page's listen cards cannot drift into
 * disagreeing about which of an artist's URLs is the one to use.
 *
 * Rungs that cannot prove themselves are absent rather than present-and-false,
 * so a caller walks what it gets and the empty array means "no player": there
 * is no rung to skip and no flag to read.
 *
 * A rung being present is a claim about the URL, not about the page behind it.
 * Whether a Bandcamp release page still resolves to a player id is only
 * answerable over the network, so a caller that needs that answer keeps walking
 * when the resolve comes back empty.
 */
export function playableMusicSources({
  bandcampUrl,
  spotifyUrl,
  bandcampScope,
}: {
  bandcampUrl?: string | null
  spotifyUrl?: string | null
  bandcampScope: BandcampScope
}): PlayableMusicSource[] {
  const sources: PlayableMusicSource[] = []

  const admitsBandcamp =
    bandcampScope === 'release' ? isBandcampReleaseUrl : isAllowedBandcampUrl
  if (bandcampUrl && admitsBandcamp(bandcampUrl)) {
    sources.push({ service: 'bandcamp', url: bandcampUrl })
  }

  const spotifyEmbed = spotifyUrl ? parseSpotifyEmbed(spotifyUrl) : null
  if (spotifyUrl && spotifyEmbed) {
    sources.push({
      service: 'spotify',
      url: spotifyUrl,
      kind: spotifyEmbed.kind,
      id: spotifyEmbed.id,
    })
  }

  return sources
}
