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
 * used to reach it. That is the desired behaviour rather than a shortcut: a
 * scraper holding last week's URL should get a card, not a 404.
 *
 * THE QUERY KEY IS AN UNBOUNDED CDN CACHE KEY, and that is an accepted cost
 * rather than an oversight. Vercel's cache keys on the full URL, so anyone can
 * force a fresh render per distinct `?w=` value, and this is the most expensive
 * response the site has (wasm instantiation, four font parses, a 1200×630 PNG
 * encode). Three things make it acceptable. It is a generic property of a CDN
 * rather than of this route — every dynamic page on the site re-renders for a
 * novel query string, this one just costs more per render. The BACKEND is not
 * amplified: every value of `w` shares one data-cache entry, so a flood costs
 * render CPU and no extra database reads. And the alternative — dropping the key
 * — trades a speculative cost for a certain defect, namely the first scraper to
 * unfurl this URL pinning its week's image against it indefinitely. The scene
 * card 404s a junk segment for the same class of reason, but that route CAN
 * reject its input; here every past date is legitimately reachable, so there is
 * nothing to reject.
 */
export default function Image() {
  return renderGraphWeekOgCard()
}
