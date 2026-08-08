import { OG_CONTENT_TYPE, OG_SIZE } from '@/lib/og/brand'
import { renderGraphWeekOgCard } from '@/features/graph/graphWeekOgCard'
import { GRAPH_WEEK_OG_ALT } from '@/features/graph/graphWeekOgLayout'

export const runtime = 'edge'
export const alt = GRAPH_WEEK_OG_ALT
export const size = OG_SIZE
export const contentType = OG_CONTENT_TYPE

/**
 * Share card for `/graph/this-week`.
 *
 * The page does NOT rely on this route's file-convention URL: that URL carries a
 * hash of the route source, so it is a constant while the card's content changes
 * every night, and third-party unfurl caches key on it. `buildGraphWeekMetadata`
 * therefore advertises this same route with the snapshot's week in a query
 * parameter. Nothing here reads that parameter — it only has to make the URL
 * vary — so this route always renders the CURRENT snapshot, whichever key was
 * used to reach it.
 */
export default function Image() {
  return renderGraphWeekOgCard()
}
