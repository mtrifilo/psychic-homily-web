/**
 * Glyph sampling + color/font helpers for the Scrying Grid hero wordmark
 * (PSY-1137). Canvas2D — the same proven approach as the approved /admin/hero-lab
 * prototype, promoted to production.
 *
 * The wordmark is rasterized to an offscreen canvas at the container's size,
 * then its alpha channel is walked on a fixed grid to produce a point cloud
 * (one "light-cell" per lit pixel). All geometry is in DEVICE pixels.
 */

const LINE_GAP_FACTOR = 0.95
const CAP_FACTOR = 0.74
const FONT_WEIGHT = 700
/** Alpha (0–255) above which a sampled pixel counts as part of a glyph. */
const ALPHA_THRESHOLD = 140

function fitFontSize(
  ctx: CanvasRenderingContext2D,
  lines: readonly string[],
  maxW: number,
  maxH: number,
  fontFamily: string,
  weight: number,
): number {
  ctx.font = `${weight} 100px ${fontFamily}`
  const widest = Math.max(1, ...lines.map((l) => ctx.measureText(l).width))
  const byWidth = (maxW / widest) * 100
  const totalFactor = (lines.length - 1) * LINE_GAP_FACTOR + CAP_FACTOR
  const byHeight = maxH / totalFactor
  return Math.max(8, Math.min(byWidth, byHeight))
}

export interface SampledPoint {
  x: number
  y: number
}

export interface SampleOpts {
  lines: readonly string[]
  fontFamily: string
  /** Sampling step in device px — smaller = denser field. */
  gapDev?: number
  /** Scale the em-box fit box (<1 = more margin around the wordmark inside
   *  the canvas). Not read with `fillInk`. */
  padScale?: number
  /** Fit and centre the glyphs' measured ink box in the whole canvas, rather
   *  than the em box in the hero's margined box. For a canvas sized to the
   *  wordmark itself, where an em-box fit clips the caps. */
  fillInk?: boolean
  /** Glyph weight; the hero samples the bold cut. */
  fontWeight?: number
}

/** Share of the canvas the ink fills with `fillInk`: under 1 so the edge
 *  cells are not cut in half. */
const INK_FILL = 0.98

/** Draw the lines so their ink box is as large as fits and centred. */
function drawInkFilled(ctx: CanvasRenderingContext2D, lines: readonly string[], fontFamily: string, weight: number) {
  const { width, height } = ctx.canvas
  ctx.font = `${weight} 100px ${fontFamily}`
  const metrics = lines.map((l) => ctx.measureText(l))
  const widest = Math.max(1, ...metrics.map((m) => m.actualBoundingBoxLeft + m.actualBoundingBoxRight))
  // The tallest ascent and deepest descent of any line, so every line fits.
  const ascent = Math.max(...metrics.map((m) => m.actualBoundingBoxAscent))
  const descent = Math.max(...metrics.map((m) => m.actualBoundingBoxDescent))
  // Ink height at 100px: that ascent, the line gaps, that descent.
  const inkHeight100 = ascent + descent + (lines.length - 1) * LINE_GAP_FACTOR * 100
  const size = Math.max(8, Math.min((width * INK_FILL) / widest, (height * INK_FILL) / Math.max(1, inkHeight100)) * 100)
  const scale = size / 100
  ctx.font = `${weight} ${size}px ${fontFamily}`
  ctx.textAlign = 'left'
  ctx.textBaseline = 'alphabetic'
  const top = (height - inkHeight100 * scale) / 2
  lines.forEach((line, i) => {
    const m = metrics[i]
    const inkWidth = (m.actualBoundingBoxLeft + m.actualBoundingBoxRight) * scale
    const x = (width - inkWidth) / 2 + m.actualBoundingBoxLeft * scale
    ctx.fillText(line, x, top + ascent * scale + i * LINE_GAP_FACTOR * size)
  })
}

/** Sample the wordmark into a list of lit points (device-px coords). */
export function sampleWordmark(widthDev: number, heightDev: number, opts: SampleOpts): SampledPoint[] {
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(widthDev))
  canvas.height = Math.max(1, Math.round(heightDev))
  // One getImageData per rebuild (rare) — willReadFrequently would force a slower
  // CPU backing store, the opposite of what a single read wants.
  const ctx = canvas.getContext('2d')
  if (!ctx) return []

  const pad = opts.padScale ?? 1
  const weight = opts.fontWeight ?? FONT_WEIGHT
  ctx.fillStyle = '#ffffff'
  if (opts.fillInk) {
    drawInkFilled(ctx, opts.lines, opts.fontFamily, weight)
  } else {
    const size = fitFontSize(ctx, opts.lines, canvas.width * 0.86 * pad, canvas.height * 0.78 * pad, opts.fontFamily, weight)
    ctx.font = `${weight} ${size}px ${opts.fontFamily}`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    const lineHeight = size * LINE_GAP_FACTOR
    const n = opts.lines.length
    for (let i = 0; i < n; i++) {
      const y = canvas.height / 2 - ((n - 1) / 2) * lineHeight + i * lineHeight
      ctx.fillText(opts.lines[i], canvas.width / 2, y)
    }
  }

  const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height)
  const gap = Math.max(2, Math.round(opts.gapDev ?? 7))
  const threshold = ALPHA_THRESHOLD
  const points: SampledPoint[] = []
  for (let y = 0; y < canvas.height; y += gap) {
    for (let x = 0; x < canvas.width; x += gap) {
      if (data[(y * canvas.width + x) * 4 + 3] > threshold) points.push({ x, y })
    }
  }
  return points
}

type WordmarkFace = Pick<SampleOpts, 'lines' | 'fontFamily' | 'fontWeight'>

/**
 * The FontFaceSet query for the face the sampler draws with: its weight, the
 * first family, and the text drawn. The first family alone, because next/font
 * lists a size-adjusted local fallback face after it that is never loaded
 * once the real face is in, and a query on the whole list never matches as
 * loaded.
 */
function faceQuery({ lines, fontFamily, fontWeight = FONT_WEIGHT }: WordmarkFace): [string, string] {
  const primaryFamily = fontFamily.split(',')[0].trim()
  return [`${fontWeight} 100px ${primaryFamily}`, lines.join(' ')]
}

/**
 * Whether that face is loaded, so a sample taken now rasterises the real
 * glyphs. True where there is no FontFaceSet, or no face of that family to
 * load; false for a query the set cannot parse.
 */
export function isWordmarkFontReady(face: WordmarkFace): boolean {
  const fonts = document.fonts
  if (!fonts?.check) return true
  try {
    return fonts.check(...faceQuery(face))
  } catch {
    return false
  }
}

/**
 * Loads that face, starting the request if nothing has asked for it yet.
 * Settles (resolves or rejects) once the face is loaded or has failed; at
 * once where there is no FontFaceSet.
 */
export function loadWordmarkFont(face: WordmarkFace): Promise<unknown> {
  const fonts = document.fonts
  if (!fonts?.load) return Promise.resolve()
  try {
    return fonts.load(...faceQuery(face))
  } catch (error) {
    return Promise.reject(error)
  }
}

/**
 * Resolve a next/font CSS variable (e.g. `--font-display`) to the concrete
 * hashed font-family string Canvas2D needs. Must be called in the browser.
 */
export function resolveFontFamily(cssVar = '--font-display'): string {
  const probe = document.createElement('span')
  probe.style.cssText = `position:absolute;visibility:hidden;font-family:var(${cssVar})`
  document.body.appendChild(probe)
  const family = getComputedStyle(probe).fontFamily || 'sans-serif'
  probe.remove()
  return family
}

export type RGB = [number, number, number]

/** Parse `#rgb` / `#rrggbb` → [r,g,b] in 0..255. */
export function parseHex(hex: string): RGB {
  let h = hex.trim().replace('#', '')
  if (h.length === 3) {
    h = h.split('').map((c) => c + c).join('')
  }
  const n = Number.parseInt(h || '000000', 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

export function rgba([r, g, b]: RGB, a: number): string {
  return `rgba(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}, ${a})`
}

export function mix(a: RGB, b: RGB, t: number): RGB {
  return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t]
}

export interface WordmarkColors {
  foreground: RGB
  primary: RGB
  isDark: boolean
}

/** Read live theme tokens off <html> (re-read on theme flip). */
export function readWordmarkColors(): WordmarkColors {
  const style = getComputedStyle(document.documentElement)
  const isDark = document.documentElement.classList.contains('dark')
  const get = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback
  return {
    foreground: parseHex(get('--foreground', isDark ? '#eee7d9' : '#1a1714')),
    primary: parseHex(get('--primary', isDark ? '#e89960' : '#d2541b')),
    isDark,
  }
}
