import { readImageHeader } from './imageSize'

/**
 * Fetch a third-party image for embedding in a share card.
 *
 * Every rejection path returns `null` and every caller is expected to fall back
 * to its text-only card. That is the same fail-open posture the card routes
 * already take for their data fetch: a share preview that renders without the
 * artwork is a far better outcome than one that 500s, because some unfurlers
 * cache the miss.
 *
 * Satori cannot be handed a URL here. `next/og` would fetch it itself, on its
 * own terms — unbounded, un-sniffed, and with no timeout — so the bytes are
 * pulled in first and passed on raw. See `RemoteImage.data` for why raw bytes
 * and not a `data:` URI.
 */

/**
 * How long a card will wait on a third party.
 *
 * The whole render sits behind this, and an unfurler that gives up first shows
 * no preview at all. Two seconds is the budget past which dropping the artwork
 * beats delaying the card.
 */
const FETCH_TIMEOUT_MS = 2000

/**
 * Transfer cap, enforced while streaming rather than after.
 *
 * `arrayBuffer()` on an attacker-chosen URL is an unbounded allocation — a
 * `Content-Length` header is a claim, and a chunked response makes no claim at
 * all.
 *
 * These are SAFETY ceilings, not targets. It is tempting to size them to the
 * plate — the 380×510 box can only ever show 193,800 pixels, so on paper a few
 * hundred KB would do. Measured against real artwork that is simply wrong: a
 * 960px-wide poster PNG on Wikimedia Commons is 1.88MB, and a 1.5MB cap
 * rejected it. Since every rejection fails open to the text-only card, a tight
 * cap does not save memory on the common path — it silently removes the
 * feature for the flyers it was built for.
 */
const MAX_BYTES = 3 * 1024 * 1024

/**
 * Decoded-pixel cap — the limit the byte cap cannot express.
 *
 * MEASURED, not estimated. The arithmetic answer — 4 bytes per pixel, so 12MP
 * is ~48MB against the edge runtime's 128MB — is wrong by about 5×, because it
 * counts only the RGBA decode and ignores what `@vercel/og` allocates around
 * it: the base64 string it builds internally, the `atob` copy, and resvg's
 * resample intermediate. Rendering real PNGs through this exact bundle and
 * watching peak RSS gives the delta over a 1×1 baseline:
 *
 *     1080×1350  (1.46MP)  →  +66MB
 *     2550×3300  (8.41MP)  →  +236MB
 *     3464×3464  (12.0MP)  →  +272MB
 *
 * So a 12MP cap admits an attacker-chosen ~2MB file that asks for nearly twice
 * the entire function budget, and an OOM is a platform kill — it goes past
 * every `catch` and every fail-open path this module is built around.
 *
 * 4MP is the largest cap with headroom, and it still clears what the feature is
 * for: the measured 960×1387 Commons poster (1.33MP) and the 1080×1350
 * Instagram export (1.46MP). Print-resolution scans and untouched phone photos
 * are above it and degrade to the text-only card, which is the correct
 * direction to be wrong — the plate only ever shows 380×510 of them anyway.
 */
const MAX_PIXELS = 4_000_000

export interface RemoteImage {
  /**
   * The raw bytes, handed to Satori as-is.
   *
   * Deliberately NOT a `data:` URI. Satori's resolver takes either, but the
   * data-URI branch ends in `Re.set(uri, …)` — a module-scoped, 100-entry LRU
   * keyed by the URI STRING. Flyer URIs are unique per show, so they are never
   * cache hits, and a warm isolate serving a crawler sweep would retain up to
   * 100 multi-megabyte strings it can never reuse. The object branch returns
   * before that `set` and retains nothing.
   *
   * An `ArrayBuffer` specifically: Satori reads dimensions via `new DataView(e)`,
   * which rejects a typed-array view.
   */
  data: ArrayBuffer
  width: number
  height: number
}

/**
 * Redirect hops allowed, each one re-validated before it is followed.
 *
 * `redirect: 'follow'` would hand the whole chain to the fetch implementation,
 * and only the FIRST url would ever have been checked — a public host that 302s
 * to `http://169.254.169.254/` would sail through every guard below. Following
 * by hand is what makes the host checks apply to the request actually sent.
 */
const MAX_REDIRECTS = 3

/**
 * Hostnames that must never be fetched, whatever a submitter typed.
 *
 * This matters more than a defence-in-depth flourish. `image_url` is writable
 * by ANY email-verified user — a show's submitter can PUT it — so the value
 * reaching this function is straightforwardly attacker-chosen.
 *
 * Addresses are classified NUMERICALLY, never by string prefix. An earlier
 * version of this guard matched `/^(10|127)\./` and friends and was defeated by
 * one URL: `https://[::ffff:169.254.169.254]/`, which the WHATWG parser
 * canonicalises to `[::ffff:a9fe:a9fe]` — no dotted quad left for any of those
 * regexes to see, and a working route to the cloud metadata service on any
 * dual-stack host. Enumerating bad prefixes loses to encoding; deciding what is
 * ALLOWED does not.
 *
 * PSY-1675 added the sibling guard this file used to ask for:
 * `backend/internal/utils/urlguard` resolves the host at the WRITE boundary and
 * refuses a name that points inward. Read the two together — neither is
 * redundant. That one has a resolver and this one does not; this one sees the
 * redirect chain and rows written before it existed, and that one does not.
 *
 * KEEP THE BLOCKED-NAME LISTS BELOW IN SYNC with `blockedHostNames` /
 * `blockedHostSuffixes` in that package. Nothing enforces it but this comment
 * and its counterpart there.
 *
 * The gap that remains in BOTH layers is DNS rebinding: a host is checked as a
 * literal here, and the write-time resolve happens once, before the answer can
 * change. Closing it needs resolve-then-pin (hand the fetcher the vetted IP,
 * not the name), which the edge runtime cannot do — it exposes no resolver.
 * What bounds the damage is that the useful half of an SSRF is shut: the
 * response is only ever rendered as an image, and anything that is not a
 * parseable PNG, JPEG or GIF is dropped before it reaches the renderer, so
 * nothing read from an internal service can come back out on the card. What
 * remains is BLIND request forgery, with a timing side channel.
 */
const BLOCKED_HOST_NAMES = new Set(['localhost', 'metadata.google.internal'])
const BLOCKED_HOST_SUFFIXES = ['.localhost', '.local', '.internal']

/**
 * Reject an IPv4 literal unless it is ordinary public unicast.
 *
 * Classified against the IANA special-purpose registry rather than the handful
 * of ranges people remember: `100.64/10` (CGNAT, routinely a container mesh),
 * `192.0.0/24` (which is where Oracle Cloud's metadata endpoint lives) and
 * `198.18/15` are all easy to omit and all reachable.
 */
function isBlockedIPv4(octets: number[]): boolean {
  const [a, b] = octets
  if (a === 0 || a === 10 || a === 127) return true
  if (a === 169 && b === 254) return true // link-local, incl. 169.254.169.254
  if (a === 172 && b >= 16 && b <= 31) return true
  if (a === 192 && b === 168) return true
  if (a === 192 && b === 0 && octets[2] === 0) return true // 192.0.0/24
  if (a === 100 && b >= 64 && b <= 127) return true // CGNAT 100.64/10
  if (a === 198 && (b === 18 || b === 19)) return true // benchmarking
  if (a >= 224) return true // multicast + reserved 240/4, and 255.255.255.255
  return false
}

/** Dotted-quad only. Anything else is a name, and names are checked separately. */
function parseIPv4(host: string): number[] | null {
  const parts = host.split('.')
  if (parts.length !== 4) return null
  const octets = parts.map(p => (/^\d{1,3}$/.test(p) ? Number(p) : NaN))
  if (octets.some(o => Number.isNaN(o) || o > 255)) return null
  return octets
}

/**
 * Allow an IPv6 literal ONLY if it is global unicast (`2000::/3`).
 *
 * Stated as one positive rule because the negative list is unbounded: this
 * single check refuses `::1`, `::`, every `::ffff:0:0/96` IPv4-mapped address,
 * `fc00::/7` unique-local, `fe80::/10` link-local and `64:ff9b::/96` NAT64
 * without naming any of them.
 */
function isAllowedIPv6(bracketed: string): boolean {
  const groups = expandIPv6(bracketed.slice(1, -1))
  if (!groups) return false
  return (groups[0] & 0xe000) === 0x2000
}

/** Expand an IPv6 literal — including a trailing embedded IPv4 — to 8 groups. */
function expandIPv6(text: string): number[] | null {
  let body = text.split('%')[0] // drop any zone id
  const embedded = body.match(/(\d{1,3}(?:\.\d{1,3}){3})$/)
  if (embedded) {
    const quad = parseIPv4(embedded[1])
    if (!quad) return null
    const hi = ((quad[0] << 8) | quad[1]).toString(16)
    const lo = ((quad[2] << 8) | quad[3]).toString(16)
    body = `${body.slice(0, embedded.index)}${hi}:${lo}`
  }

  const halves = body.split('::')
  if (halves.length > 2) return null
  const head = halves[0] ? halves[0].split(':') : []
  const tail = halves.length === 2 ? (halves[1] ? halves[1].split(':') : []) : []
  const fill = halves.length === 2 ? 8 - head.length - tail.length : 0
  if (fill < 0) return null
  const parts = [...head, ...Array<string>(fill).fill('0'), ...tail]
  if (parts.length !== 8) return null

  const groups = parts.map(p => (/^[0-9a-f]{1,4}$/i.test(p) ? parseInt(p, 16) : NaN))
  return groups.some(Number.isNaN) ? null : groups
}

/**
 * `https` only, and not pointed anywhere private.
 *
 * The scheme check is doing more than it looks: it is what excludes `file:`,
 * `data:` and the assorted schemes a fetch implementation may still honour, and
 * plain `http` is dropped with them because the only images worth putting on a
 * public card are ones served over TLS anyway.
 */
function isFetchableImageUrl(raw: string): URL | null {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return null
  }
  if (url.protocol !== 'https:') return null

  // The trailing dot is stripped BEFORE any name match. It is a valid absolute
  // FQDN that resolvers honour, and `https://localhost./` slipped every
  // name-based check below by adding one character.
  const host = url.hostname.toLowerCase().replace(/\.$/, '')
  if (!host) return null

  if (host.startsWith('[')) return isAllowedIPv6(host) ? url : null

  const ipv4 = parseIPv4(host)
  if (ipv4) return isBlockedIPv4(ipv4) ? null : url

  if (BLOCKED_HOST_NAMES.has(host)) return null
  if (BLOCKED_HOST_SUFFIXES.some(suffix => host.endsWith(suffix))) return null
  return url
}

/**
 * Why a flyer did not make it onto the card.
 *
 * The distinction is a CACHING one, and it matters more than it looks. A card
 * without its flyer is held for `OG_FALLBACK_CACHE_SECONDS` so a momentary blip
 * is not baked in for a day — but `rejected` outcomes are not blips. An `http:`
 * URL (which the backend accepts and this module does not), a WebP flyer, a
 * 5MP scan: those are permanent facts about the row. Treating them as transient
 * would re-render that show's card every 60 seconds forever, booting the edge
 * function and re-instantiating the resvg wasm each time — exactly the cost
 * `ogCacheControl` exists to avoid.
 */
export type FlyerOutcome =
  | { status: 'none' }
  /** Fetched, sniffed and sized. */
  | { status: 'ok'; image: RemoteImage }
  /** Permanently unusable as submitted — will not become usable on retry. */
  | { status: 'rejected' }
  /** Transient: timeout, connection failure, 5xx. Worth retrying soon. */
  | { status: 'unavailable' }

export async function loadRemoteImage(raw: string | null | undefined): Promise<FlyerOutcome> {
  if (!raw) return { status: 'none' }
  const url = isFetchableImageUrl(raw)
  if (!url) return { status: 'rejected' }

  try {
    // One deadline for the whole chase, not one per hop: three sequential
    // two-second hops would be a six-second card.
    const deadline = AbortSignal.timeout(FETCH_TIMEOUT_MS)
    const res = await fetchFollowingValidatedRedirects(url, deadline)
    if (!res) return { status: 'rejected' } // refused a hop, or ran out of them
    if (!res.ok || !res.body) {
      await discard(res)
      // 5xx and 429 are the server having a bad moment; a 404 or a 403 is the
      // link being wrong, which no amount of retrying fixes.
      return { status: res.status >= 500 || res.status === 429 ? 'unavailable' : 'rejected' }
    }

    // A latency optimisation, NOT the safety guard: it saves pulling a huge
    // body from an honest server, but the header is a claim and a chunked
    // response makes none, so `readCapped` below is what actually bounds this.
    const declared = Number(res.headers.get('content-length'))
    if (Number.isFinite(declared) && declared > MAX_BYTES) {
      await discard(res)
      return { status: 'rejected' }
    }

    const bytes = await readCapped(res.body, MAX_BYTES)
    if (!bytes) return { status: 'rejected' }

    const header = readImageHeader(bytes)
    if (!header) return { status: 'rejected' }
    if (header.width < 1 || header.height < 1) return { status: 'rejected' }
    if (header.width * header.height > MAX_PIXELS) return { status: 'rejected' }

    return {
      status: 'ok',
      image: { data: bytes.buffer as ArrayBuffer, width: header.width, height: header.height },
    }
  } catch {
    // Timeout, DNS failure, TLS failure, connection reset — none of them worth
    // a Sentry event: a broken third-party image URL is a data-quality fact
    // about one show, not an incident, and reporting it would turn every stale
    // flyer link into recurring noise on every unfurl.
    return { status: 'unavailable' }
  }
}

/** Release a body we have decided not to read, rather than leaving it in flight. */
async function discard(res: Response): Promise<void> {
  await res.body?.cancel().catch(() => {})
}

/**
 * An explicit list, matching `lib/sitemap-monitor/fetch.ts` — the repo's other
 * manual redirect walker. Treating the whole 3xx block as a redirect would put
 * a 304 or a 300 down the follow path, where they have no `Location` to follow
 * and would be reported as a failed hop rather than returned to the caller.
 */
function isRedirect(status: number): boolean {
  return status === 301 || status === 302 || status === 303 || status === 307 || status === 308
}

/**
 * Walk the redirect chain by hand, running every hop through the same host
 * checks as the original URL.
 *
 * A response body is only ever returned for a final, non-redirect hop; the 3xx
 * responses are cancelled as they go so their connections are not left open.
 */
async function fetchFollowingValidatedRedirects(
  start: URL,
  signal: AbortSignal
): Promise<Response | null> {
  let target = start
  for (let hop = 0; hop <= MAX_REDIRECTS; hop++) {
    const res = await fetch(target, {
      signal,
      redirect: 'manual',
      // The image is public by definition; sending anything ambient would only
      // widen what a submitted URL can reach.
      credentials: 'omit',
      referrerPolicy: 'no-referrer',
      // Narrower than `image/*` on purpose: a content-negotiating CDN offered
      // the wildcard will happily return WebP for a URL that would otherwise
      // have served JPEG, and WebP is a format this path must then drop.
      headers: { accept: 'image/png,image/jpeg,image/gif' },
    })

    if (!isRedirect(res.status)) return res

    const location = res.headers.get('location')
    res.body?.cancel().catch(() => {})
    // An opaque redirect — no readable `location` — cannot be validated, so it
    // is not followed. Dropping the artwork beats following a blind hop.
    if (!location) return null
    // Resolved against the current hop, since `location` may be relative.
    const next = isFetchableImageUrl(new URL(location, target).href)
    if (!next) return null
    target = next
  }
  return null
}

/**
 * Read a body, giving up the moment it exceeds `limit`.
 *
 * Returns `null` rather than a truncated buffer: half an image is not an image,
 * and a partial header would parse into dimensions that describe a file we do
 * not have.
 */
async function readCapped(body: ReadableStream<Uint8Array>, limit: number): Promise<Uint8Array | null> {
  const reader = body.getReader()
  const chunks: Uint8Array[] = []
  let total = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      total += value.byteLength
      if (total > limit) return null
      chunks.push(value)
    }
  } finally {
    // Releases the connection on the over-limit path, where the body is
    // abandoned mid-stream.
    reader.cancel().catch(() => {})
  }

  // Exactly-sized, so `.buffer` can be handed onward without a further copy or
  // an offset/length the consumer would have to respect.
  const out = new Uint8Array(total)
  let at = 0
  for (const chunk of chunks) {
    out.set(chunk, at)
    at += chunk.byteLength
  }
  return out
}
