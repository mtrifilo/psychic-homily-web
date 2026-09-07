/**
 * The scene page's contribution-hook copy: decisions about MEANING, not markup,
 * so the sentence is testable without a render. Same split as `sceneRooms.ts`
 * and `sceneNewBands.ts`.
 */

/**
 * `11 Phoenix bands have no listen link → Help finish Phoenix`, printed in the
 * accent register's micro-caps.
 *
 * "Listen link", not the locked mock's "Bandcamp link". The count is over bands
 * with none of spotify, bandcamp, youtube or soundcloud, so naming one platform
 * would claim a set the number does not describe: a band with a Spotify page
 * and no Bandcamp is not counted. The four columns are the project's definition
 * of a listen link, spelled in `noListenLinkSQL`
 * (services/catalog/scene_gaps.go); a platform outside them clears nothing, so
 * the sentence overstates a band reachable only on, say, Apple Music.
 *
 * The city is the SCENE's, not the one the gaps payload echoes back, so the
 * sentence names the page the reader is on.
 */
export function gapLineCopy(count: number, city: string): string {
  const subject =
    count === 1 ? `1 ${city} band has` : `${count} ${city} bands have`
  return `${subject} no listen link → Help finish ${city}`
}
