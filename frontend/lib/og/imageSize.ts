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
 *    A byte cap does not bound decoded size — JPEG compresses ~100:1, so a
 *    400KB download can be a 12000×12000 canvas that costs ~576MB to
 *    rasterise. Reading the header first makes the pixel budget cheap to
 *    enforce, and a response that parses as no known format is rejected
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
 * That rules out WebP and AVIF specifically. WebP is common on the web, so a
 * WebP flyer silently getting the text card is a real (if minor) product gap —
 * the fix is to transcode at the write boundary, not to widen this.
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
 * The walk is bounded by the buffer length, and every segment advance is at
 * least two bytes, so a malformed file terminates rather than spinning.
 */
function readJpeg(b: Uint8Array): ImageHeader | null {
  if (b.length < 4 || b[0] !== 0xff || b[1] !== 0xd8) return null

  let at = 2
  while (at + 9 < b.length) {
    if (b[at] !== 0xff) {
      // Fill bytes are legal between segments; anything else means the chain
      // is broken and further offsets would be nonsense.
      at++
      continue
    }
    const marker = b[at + 1]
    // Standalone markers carry no length field.
    if (marker === 0xd8 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
      at += 2
      continue
    }
    if (marker === 0xd9 || marker === 0xda) return null // end of image / scan data
    const length = u16be(b, at + 2)
    if (length < 2) return null
    // SOF0..SOF15, minus the four that are not frame headers (DHT, JPGA, DAC,
    // and the restart-interval-adjacent 0xCC).
    const isSof =
      marker >= 0xc0 &&
      marker <= 0xcf &&
      marker !== 0xc4 &&
      marker !== 0xc8 &&
      marker !== 0xcc
    if (isSof) {
      return { width: u16be(b, at + 7), height: u16be(b, at + 5), mime: 'image/jpeg' }
    }
    at += 2 + length
  }
  return null
}
