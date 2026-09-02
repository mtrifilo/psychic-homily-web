import { isAllowedBandcampUrl } from '@/lib/bandcamp'
import { parseSpotifyEmbed } from '@/lib/spotify'

/**
 * Could these inputs produce anything at all in a `<MusicEmbed>`?
 *
 * Callers render their own heading above that component ("Top tracks", a LISTEN
 * block), so each has to decide whether to render the section BEFORE the
 * component decides what goes in it. Five of them answered that by testing the
 * stored column for truthiness, which was correct while any Bandcamp URL yielded
 * at least a link. The outbound-link gate (PSY-1966) makes that premise false, so
 * the question lives here and the callers ask it instead of restating it. A
 * headed section with no player under it is the failure mode this prevents
 * (PSY-1302).
 *
 * In lib rather than beside the component because most callers mock
 * `@/components/shared` in their tests; a predicate exported from there would
 * vanish under the mock and take the caller's own logic with it.
 *
 * It must stay in step with MusicEmbed's `deriveEmbedState`, which is why both
 * read the SAME two predicates (isAllowedBandcampUrl, parseSpotifyEmbed) rather
 * than each spelling the rule out.
 *
 * NECESSARY, not sufficient, and deliberately so. The album URL is tested with
 * `isAllowedBandcampUrl` rather than the stricter `isBandcampReleaseUrl` because
 * `/api/bandcamp/album-id` gates on the host anchor alone: an on-platform page
 * that is not a release can still resolve to a playable iframe, and predicting
 * that here would need the fetch this function must not do. What remains is a
 * legacy on-platform non-release row whose page carries no player, which shows a
 * headed empty section. Closing that needs the heading to move inside the
 * component.
 */
export function hasRenderableMusic({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
}: {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
}): boolean {
  if (bandcampAlbumUrl && isAllowedBandcampUrl(bandcampAlbumUrl)) return true
  if (bandcampProfileUrl && isAllowedBandcampUrl(bandcampProfileUrl)) return true
  return Boolean(spotifyUrl && parseSpotifyEmbed(spotifyUrl))
}
