/**
 * The scene page's contribution-hook copy: decisions about MEANING, not markup,
 * so the sentence is testable without a render. Same split as `sceneRooms.ts`
 * and `sceneNewBands.ts`.
 */

/**
 * `11 Phoenix bands have no listen link → Help finish Phoenix`, printed in the
 * accent register's micro-caps.
 *
 * "Listen link", not the locked mock's "Bandcamp link". `SceneGapsResponse`
 * documents the population: bands with no music-platform link at all. Naming
 * one platform would claim a set the number does not describe, since a band
 * with a Spotify page and no Bandcamp is not counted.
 *
 * The city is the SCENE's, not the one the gaps payload echoes back, so the
 * sentence names the page the reader is on.
 */
export function gapLineCopy(count: number, city: string): string {
  const subject =
    count === 1 ? `1 ${city} band has` : `${count} ${city} bands have`
  return `${subject} no listen link → Help finish ${city}`
}
