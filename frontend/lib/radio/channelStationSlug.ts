/**
 * The station slug a `/radio/{network}/channel/{local}` URL names: the local
 * segment with the network's `{network}-` prefix put back, unless it already
 * carries it (`/radio/wfmu/channel/drummer` names `wfmu-drummer`). The reverse
 * of the stripping `getStationDetailUrl` in features/radio does.
 *
 * Here rather than beside the channel page because `proxy.ts` resolves the
 * same station before the page renders, and the two must agree.
 */
export function channelStationSlug(networkSlug: string, localSlug: string): string {
  const prefix = `${networkSlug}-`
  return localSlug.startsWith(prefix) ? localSlug : `${prefix}${localSlug}`
}
