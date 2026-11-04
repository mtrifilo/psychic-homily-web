/**
 * The one definition of a playlist row's anchor. The episode page renders
 * `id={playAnchorId(play.id)}` on each row and reads the same shape back
 * from the URL hash; "As heard on" links write it. All three must agree, so
 * none of them spells the format themselves.
 */
export function playAnchorId(playId: number): string {
  return `play-${playId}`
}

/** `/radio/{station}/{show}/{air_date}#play-{id}`: one play on its episode page. */
export function episodePlayHref(
  stationSlug: string,
  showSlug: string,
  airDate: string,
  playId: number
): string {
  return `/radio/${stationSlug}/${showSlug}/${airDate}#${playAnchorId(playId)}`
}

const PLAY_ANCHOR_HASH = /^#play-(\d+)$/

/** The play id a `#play-{id}` hash names, or null for any other hash. */
export function parsePlayAnchor(hash: string): number | null {
  const match = PLAY_ANCHOR_HASH.exec(hash)
  if (!match) return null
  const id = Number(match[1])
  return Number.isSafeInteger(id) ? id : null
}
