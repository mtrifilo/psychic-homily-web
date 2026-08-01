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
 * all. 3MB comfortably covers a photographed gig poster.
 */
const MAX_BYTES = 3 * 1024 * 1024

/**
 * Decoded-pixel cap — the limit the byte cap cannot express.
 *
 * Rasterising costs roughly 4 bytes per pixel regardless of how well the file
 * compressed, so a small JPEG of a 12000×12000 canvas would ask for ~576MB in a
 * runtime that has a small fraction of that. 12MP (about 4000×3000) is far
 * beyond any flyer and stays inside the budget.
 */
const MAX_PIXELS = 12_000_000

export interface RemoteImage {
  /** `data:` URI, with the MIME sniffed from the magic bytes. */
  dataUri: string
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

    // A truthful `Content-Length` lets an oversized response be dropped before
    // a single byte is buffered. An absent or lying one is caught below.
    const declared = Number(res.headers.get('content-length'))
    if (Number.isFinite(declared) && declared > MAX_BYTES) return null

    const bytes = await readCapped(res.body, MAX_BYTES)
    if (!bytes) return null

    const header = readImageHeader(bytes)
    if (!header) return null
    if (header.width < 1 || header.height < 1) return null
    if (header.width * header.height > MAX_PIXELS) return null

    return {
      dataUri: `data:${header.mime};base64,${toBase64(bytes)}`,
      width: header.width,
      height: header.height,
    }
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

    if (res.status < 300 || res.status > 399) return res

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

  const out = new Uint8Array(total)
  let at = 0
  for (const chunk of chunks) {
    out.set(chunk, at)
    at += chunk.byteLength
  }
  return out
}

/**
 * `btoa` needs a binary string, and spreading megabytes into
 * `String.fromCharCode` at once overflows the call stack, so it is chunked.
 */
function toBase64(bytes: Uint8Array): string {
  const CHUNK = 0x8000
  let binary = ''
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(binary)
}
