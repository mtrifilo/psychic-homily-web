import { isAllowedBandcampUrl } from '@/lib/bandcamp'
import { parseSpotifyEmbed } from '@/lib/spotify'

/**
 * Could these inputs produce anything at all in a `<MusicEmbed>`?
 *
 * Callers render their own heading above that component ("Top tracks", a LISTEN
 * block), so each has to decide whether to render the section BEFORE the
 * component decides what goes in it. A headed section with no player under it is
 * the failure mode this prevents (PSY-1302); a stored Bandcamp URL alone does
 * not answer the question, because the component refuses to render one it cannot
 * prove is Bandcamp.
 *
 * This is a RESTATEMENT of `MusicEmbed`'s own entry conditions, not a call into
 * it — a component cannot answer a question asked before it mounts, and most
 * callers mock `@/components/shared` in their tests, so a predicate exported
 * from there would vanish under the mock. What keeps the two in step is that
 * they gate on the same imported predicates, and that
 * `MusicEmbed.test.tsx` asserts the containment directly: whatever this calls
 * unrenderable, the component renders as nothing.
 *
 * NECESSARY, not sufficient, and deliberately so:
 *
 *   - The album URL is tested with `isAllowedBandcampUrl`, the same host anchor
 *     `MusicEmbed` applies before it will even ask the resolver, and the same one
 *     `/api/bandcamp/album-id` enforces before it fetches. Using the stricter
 *     `isBandcampReleaseUrl` here would hide sections that do render: an
 *     on-platform page that is not a release can still resolve to a playable id.
 *   - What that leaves is an on-platform NON-release row whose page carries no
 *     player: true here, nothing rendered there, so the heading still strands.
 *     Predicting it would need the fetch this function must not do. Closing it
 *     needs the heading to move inside the component.
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
