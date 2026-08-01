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
 * pulled in first and passed on as a data URI.
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
 * Rasterising costs roughly 4 bytes per pixel however well the file compressed,
 * so a small JPEG declaring a 12000×12000 canvas would ask for ~576MB in a
 * runtime with a small fraction of that.
 *
 * 12MP is where the runtime runs out, not where flyers do: at 4 bytes a pixel
 * this already admits a ~48MB decode, and the edge runtime has 128MB for that
 * plus the resvg wasm, the fonts and the 1200×630 output.
 *
 * Be precise about what that excludes, because it is close to the line: a
 * hosted flyer (a 1080×1350 Instagram export, the measured 960×1387 poster) is
 * far under it, but a full-resolution phone photo at 4032×3024 is 12.19MP —
 * just OVER, and it falls back to the text-only card. That is the right way to
 * be wrong: image hosts downscale, so URLs pointing at untouched 12MP
 * originals are rare, and the alternative is an OOM that takes the route down
 * rather than degrading it.
 */
const MAX_PIXELS = 12_000_000

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
 * by ANY email-verified user — a show's submitter can PUT it, and the backend
 * validates only the scheme and a 2048-char cap — so the value reaching this
 * function is straightforwardly attacker-chosen, and this is the only place it
 * is checked before a request goes out.
 *
 * The one gap left is DNS: a host is checked as a literal, so an attacker-owned
 * name that RESOLVES to a private address still passes. Closing that needs
 * resolve-then-pin, which the edge runtime cannot do — it exposes no resolver.
 * What bounds the damage is that the useful half of an SSRF is already shut:
 * the response is only ever rendered as an image, and anything that is not a
 * parseable JPEG, PNG, WebP or GIF is dropped before it reaches the renderer,
 * so nothing read from an internal service can come back out on the card.
 * Blind request forgery against a resolvable-to-private name remains possible,
 * and the durable fix for it belongs at the write boundary in the backend.
 */
const BLOCKED_HOSTS = /^(localhost|(\[::1\])|0\.0\.0\.0|metadata\.google\.internal)$/i
const BLOCKED_HOST_SUFFIXES = ['.localhost', '.local', '.internal']

/**
 * `https` only, and not pointed at ourselves.
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
  const host = url.hostname.toLowerCase()
  if (BLOCKED_HOSTS.test(host)) return null
  if (BLOCKED_HOST_SUFFIXES.some(suffix => host.endsWith(suffix))) return null
  // Literal private/link-local IPv4, plus the IPv6 unique-local and link-local
  // ranges. Hosts that merely resolve into these are not covered — see above.
  if (/^(10|127)\./.test(host)) return null
  if (/^192\.168\./.test(host)) return null
  if (/^169\.254\./.test(host)) return null
  if (/^172\.(1[6-9]|2\d|3[01])\./.test(host)) return null
  if (/^\[(f[cd][0-9a-f]{2}|fe80):/i.test(url.host)) return null
  return url
}

export async function loadRemoteImage(raw: string | null | undefined): Promise<RemoteImage | null> {
  if (!raw) return null
  const url = isFetchableImageUrl(raw)
  if (!url) return null

  try {
    // One deadline for the whole chase, not one per hop: three sequential
    // two-second hops would be a six-second card.
    const deadline = AbortSignal.timeout(FETCH_TIMEOUT_MS)
    const res = await fetchFollowingValidatedRedirects(url, deadline)
    if (!res?.ok || !res.body) return null

    // A latency optimisation, NOT the safety guard: it saves pulling a huge
    // body from an honest server, but the header is a claim and a chunked
    // response makes none, so `readCapped` below is what actually bounds this.
    const declared = Number(res.headers.get('content-length'))
    if (Number.isFinite(declared) && declared > MAX_BYTES) return null

    const bytes = await readCapped(res.body, MAX_BYTES)
    if (!bytes) return null

    const header = readImageHeader(bytes)
    if (!header) return null
    if (header.width < 1 || header.height < 1) return null
    if (header.width * header.height > MAX_PIXELS) return null

    return { data: bytes.buffer as ArrayBuffer, width: header.width, height: header.height }
  } catch {
    // Timeout, DNS failure, TLS failure, connection reset — all the same
    // outcome for the caller, and none of them worth a Sentry event: a broken
    // third-party image URL is a data-quality fact about one show, not an
    // incident, and reporting it would turn every stale flyer link into
    // recurring noise on every unfurl.
    return null
  }
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
      headers: { accept: 'image/*' },
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
