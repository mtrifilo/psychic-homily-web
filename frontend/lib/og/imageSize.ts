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
 *
 * Kept dependency-free: this is imported into an edge bundle, where OG failures
 * surface at DEPLOY time rather than at build, so an image-parsing library is
 * exactly the kind of weight not worth carrying for four format headers.
 */

export interface ImageHeader {
  width: number
  height: number
  /** Derived from the magic bytes, never from the response's `Content-Type`. */
  mime: string
}

/**
 * Read the intrinsic size of a JPEG, PNG, WebP or GIF, or `null` for anything
 * else — including a truncated file of a format we do support.
 *
 * These four cover what image hosts serve. AVIF and HEIC are deliberately out:
 * their sizes live inside an ISOBMFF box tree that needs a real walker, and
 * Satori's rasteriser does not decode them anyway, so parsing one would only
 * produce a card with a blank plate.
 */
export function readImageHeader(bytes: Uint8Array): ImageHeader | null {
  return readPng(bytes) ?? readGif(bytes) ?? readWebp(bytes) ?? readJpeg(bytes)
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

function u24le(b: Uint8Array, at: number): number {
  return b[at] | (b[at + 1] << 8) | (b[at + 2] << 16)
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
 * WebP has three container flavours and each stores its size differently.
 *
 * Lossy (`VP8 `) keeps it in the frame header; lossless (`VP8L`) packs two
 * 14-bit minus-one fields into a bitstream; extended (`VP8X`) — which is what
 * an animated or alpha-channel flyer arrives as — carries 24-bit minus-one
 * canvas fields in the chunk itself.
 */
function readWebp(b: Uint8Array): ImageHeader | null {
  if (b.length < 30) return null
  if (!matches(b, 0, 'RIFF') || !matches(b, 8, 'WEBP')) return null

  if (matches(b, 12, 'VP8X')) {
    return {
      width: u24le(b, 24) + 1,
      height: u24le(b, 27) + 1,
      mime: 'image/webp',
    }
  }
  if (matches(b, 12, 'VP8L')) {
    if (b[20] !== 0x2f) return null // signature byte of the lossless bitstream
    // Little-endian bit packing: width is the low 14 bits, height the next 14.
    const packed = (b[21] | (b[22] << 8) | (b[23] << 16) | (b[24] << 24)) >>> 0
    return {
      width: (packed & 0x3fff) + 1,
      height: ((packed >>> 14) & 0x3fff) + 1,
      mime: 'image/webp',
    }
  }
  if (matches(b, 12, 'VP8 ')) {
    // 0x9d012a is the start code that precedes the two 14-bit dimensions.
    if (b[23] !== 0x9d || b[24] !== 0x01 || b[25] !== 0x2a) return null
    return {
      width: u16le(b, 26) & 0x3fff,
      height: u16le(b, 28) & 0x3fff,
      mime: 'image/webp',
    }
  }
  return null
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
