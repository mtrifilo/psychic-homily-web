import { permanentRedirect } from 'next/navigation'

/**
 * PSY-1457 cutover: the Observatory now owns discovery at /graph. Keep the
 * long-lived /explore URL as a permanent app-level handoff instead of leaving
 * two competing discovery shells alive.
 */
export default function ExploreRedirect() {
  permanentRedirect('/graph')
}
