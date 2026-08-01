/**
 * Intrinsic dimensions and format, read from an image's own header bytes.
 *
 * Two jobs, both of which have to happen BEFORE the bytes reach the renderer:
 *
 * 1. **Aspect ratio.** A flyer plate is letterboxed at its natural ratio, so
 *    the layout needs the real pixel dimensions to compute the box. Deriving
 *    them here rather than leaning on CSS `object-fit` keeps the geometry a
 *    pure function the layout tests can assert against, instead of a behaviour
 *    of Satori's image resolver.
 * 2. **A sniff that the bytes are actually an image, and how big it decodes.**
 *    A byte cap does not bound decoded size. A flat-colour PNG of a 12000×12000
 *    canvas compresses to tens of KB and still asks for ~576MB of RGBA to
 *    rasterise, so a download small enough to pass any byte cap can still
 *    exhaust the runtime. Reading the header first makes the pixel budget cheap
 *    to enforce, and a response that parses as no known format is rejected
 *    without ever being handed to the renderer.
 *
 * The MIME type comes from the magic bytes rather than the server's
 * `Content-Type`, which is remote-controlled and may simply be wrong.
 */

export interface ImageHeader {
  width: number
  height: number
  /** Derived from the magic bytes, never from the response's `Content-Type`. */
  mime: string
}

/**
 * Read the intrinsic size of a PNG, GIF or JPEG, or `null` for anything else —
 * including a truncated file of a format we do support.
 *
 * The accepted set is exactly what the card's rasteriser can draw, and that is
 * load-bearing rather than incidental. Satori's supported list is
 * `[png, apng, jpeg, gif, svg]`; handed anything else it THROWS
 * `Unsupported image type` from inside `ImageResponse`, which would 500 the
 * route — destroying the fail-open guarantee this whole path exists to
 * provide. So a format this parser accepts but the renderer cannot draw is
 * strictly worse than one it rejects: rejecting yields the text-only card,
 * accepting yields a broken share preview.
 *
 * That rules out WebP and AVIF, which the rasteriser genuinely cannot draw.
 * WebP is common on the web, so a WebP flyer silently getting the text card is
 * a real (if minor) product gap — the fix is to transcode at the write
 * boundary, not to widen this.
 *
 * SVG is excluded too, and for a different reason: the rasteriser DOES draw it,
 * but the bytes here come from a URL any email-verified user can set, and an
 * SVG is a document — a script-and-external-reference-bearing format handed to
 * a renderer. That is a materially larger attack surface than a raster header,
 * for a format essentially no gig flyer uses. Do not "fix" this by adding it.
 *
 * Kept dependency-free: this is imported into an edge bundle, where OG failures
 * surface at DEPLOY time rather than at build, so an image-parsing library is
 * exactly the kind of weight not worth carrying for three format headers.
 */
export function readImageHeader(bytes: Uint8Array): ImageHeader | null {
  return readPng(bytes) ?? readGif(bytes) ?? readJpeg(bytes)
}

function u32be(b: Uint8Array, at: number): number {
  return ((b[at] << 24) | (b[at + 1] << 16) | (b[at + 2] << 8) | b[at + 3]) >>> 0
}

function u16be(b: Uint8Array, at: number): number {
  return (b[at] << 8) | b[at + 1]
}

function u16le(b: Uint8Array, at: number): number {
  return b[at] | (b[at + 1] << 8)
}

function matches(b: Uint8Array, at: number, ascii: string): boolean {
  for (let i = 0; i < ascii.length; i++) {
    if (b[at + i] !== ascii.charCodeAt(i)) return false
  }
  return true
}

const PNG_MAGIC = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]

/** IHDR is required to be the first chunk, at a fixed offset. */
function readPng(b: Uint8Array): ImageHeader | null {
  if (b.length < 24) return null
  for (let i = 0; i < PNG_MAGIC.length; i++) {
    if (b[i] !== PNG_MAGIC[i]) return null
  }
  if (!matches(b, 12, 'IHDR')) return null
  return { width: u32be(b, 16), height: u32be(b, 20), mime: 'image/png' }
}

function readGif(b: Uint8Array): ImageHeader | null {
  if (b.length < 10) return null
  if (!matches(b, 0, 'GIF87a') && !matches(b, 0, 'GIF89a')) return null
  return { width: u16le(b, 6), height: u16le(b, 8), mime: 'image/gif' }
}

/**
 * JPEG stores its size in a Start-Of-Frame marker whose offset depends on
 * everything before it, so the segment chain has to be walked.
 *
 * This walk is deliberately a STRICT SUBSET of the rasteriser's, not a correct
 * reading of T.81, because the two disagreeing is what breaks the route. Satori
 * begins its own walk at offset 4 and returns only for SOF0/SOF1/SOF2; handed
 * anything else it throws, from inside the response stream where nothing can
 * catch it. Accepting a JPEG it will reject is therefore worse than rejecting a
 * valid one — the first is a broken share preview, the second is the text card.
 *
 * Concretely that means three narrowings against the spec:
 *
 * - The segment at offset 2 is never a candidate. Satori's loop starts at the
 *   first LENGTH field and reads the marker of the segment after it, so a file
 *   whose SOF comes first is sizeable here and invisible to Satori — which then
 *   throws. Real encoders emit APP0/APP1 first, so this costs nothing.
 * - SOF0/1/2 only (baseline, extended, progressive). Lossless and arithmetic
 *   frames are real JPEG and Satori cannot size them.
 * - Every segment must carry a length, and a byte that is not `0xFF` where a
 *   marker belongs ENDS the walk. Satori has no concept of standalone markers,
 *   and resyncing past a bad byte accepts arbitrary junk with a plausible SOF
 *   buried in it.
 *
 * The walk is bounded by the buffer length and every advance is at least two
 * bytes, so a malformed file terminates rather than spinning.
 */
function readJpeg(b: Uint8Array): ImageHeader | null {
  if (b.length < 4 || b[0] !== 0xff || b[1] !== 0xd8) return null

  // `at` is the offset of the CURRENT segment's marker. Satori's loop counter
  // is its length field (`at + 2`), and it only ever inspects the marker of the
  // segment AFTER the one it is standing on — which is why the segment at
  // offset 2 can never be the SOF as far as it is concerned.
  let at = 2
  while (at + 9 < b.length) {
    if (b[at] !== 0xff) return null
    const length = u16be(b, at + 2)
    if (length < 2) return null

    const next = at + 2 + length
    // Every segment must carry a length for this walk to stay in step, and the
    // next marker must land inside the buffer with room for a frame header.
    if (next + 9 > b.length) return null
    if (b[next] !== 0xff) return null

    const marker = b[next + 1]
    if (marker === 0xc0 || marker === 0xc1 || marker === 0xc2) {
      return { width: u16be(b, next + 7), height: u16be(b, next + 5), mime: 'image/jpeg' }
    }
    at = next
  }
  return null
}
