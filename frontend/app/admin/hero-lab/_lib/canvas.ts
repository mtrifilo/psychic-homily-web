/**
 * Hero Lab — Canvas2D text-sampling + color helpers shared by the effects.
 *
 * All geometry here is in DEVICE pixels (canvas backing-store coordinates):
 * effects size their canvas to `cssWidth * dpr` and draw 1:1 without scaling
 * the context, so sampled points, the wordmark image, and pointer coordinates
 * all live in the same device-pixel space. Keeping one coordinate system avoids
 * the classic dpr double-scaling bugs.
 */

export interface SampledPoint {
  x: number
  y: number
}

/** Vertical rhythm used when stacking wordmark lines. */
const LINE_GAP_FACTOR = 0.95
/** Rough cap-height as a fraction of font size, for vertical centering math. */
const CAP_FACTOR = 0.74

/** Largest font size (device px) that fits the lines within the given box. */
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

export interface WordmarkDrawOpts {
  lines: readonly string[]
  fontFamily: string
  weight?: number
  color: string
}

/**
 * Render the wordmark, centered, onto a fresh offscreen canvas (device px).
 * Used directly by the image-displacement effects (mirage / scrying pool) and
 * indirectly by the point sampler below.
 */
export function makeWordmarkCanvas(
  widthDev: number,
  heightDev: number,
  opts: WordmarkDrawOpts,
): HTMLCanvasElement {
  const weight = opts.weight ?? 700
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, widthDev)
  canvas.height = Math.max(1, heightDev)
  const ctx = canvas.getContext('2d')
  if (!ctx) return canvas
  const size = fitFontSize(ctx, opts.lines, widthDev * 0.86, heightDev * 0.8, opts.fontFamily, weight)
  ctx.font = `${weight} ${size}px ${opts.fontFamily}`
  ctx.fillStyle = opts.color
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const lineHeight = size * LINE_GAP_FACTOR
  const n = opts.lines.length
  for (let i = 0; i < n; i++) {
    const y = heightDev / 2 - ((n - 1) / 2) * lineHeight + i * lineHeight
    ctx.fillText(opts.lines[i], widthDev / 2, y)
  }
  return canvas
}

export interface SampleResult {
  points: SampledPoint[]
  /** Fitted font size in device px (handy for scaling effect parameters). */
  size: number
}

/**
 * Sample the wordmark into a list of "lit" points by rasterizing it white on
 * transparent and walking the alpha channel on a fixed grid.
 */
export function sampleWordmark(
  widthDev: number,
  heightDev: number,
  opts: Omit<WordmarkDrawOpts, 'color'> & { gapDev?: number; alphaThreshold?: number },
): SampleResult {
  const weight = opts.weight ?? 700
  const canvas = makeWordmarkCanvas(widthDev, heightDev, { ...opts, color: '#ffffff' })
  const ctx = canvas.getContext('2d')
  if (!ctx) return { points: [], size: 0 }
  const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height)
  const gap = Math.max(2, Math.round(opts.gapDev ?? 7))
  const threshold = opts.alphaThreshold ?? 140
  const points: SampledPoint[] = []
  for (let y = 0; y < canvas.height; y += gap) {
    for (let x = 0; x < canvas.width; x += gap) {
      if (data[(y * canvas.width + x) * 4 + 3] > threshold) points.push({ x, y })
    }
  }
  const size = fitFontSize(ctx, opts.lines, widthDev * 0.86, heightDev * 0.8, opts.fontFamily, weight)
  return { points, size }
}

export type RGB = [number, number, number]

/** Parse `#rgb` / `#rrggbb` into an [r,g,b] triple (0–255). */
export function parseHex(hex: string): RGB {
  let h = hex.trim().replace('#', '')
  if (h.length === 3) {
    h = h
      .split('')
      .map((ch) => ch + ch)
      .join('')
  }
  const n = Number.parseInt(h || '000000', 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

export function rgba([r, g, b]: RGB, a: number): string {
  return `rgba(${Math.round(r)}, ${Math.round(g)}, ${Math.round(b)}, ${a})`
}

/** Linear interpolate between two colors. */
export function mix(a: RGB, b: RGB, t: number): RGB {
  return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t]
}

export function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v
}

/** Unnormalized gaussian falloff — 1 at d=0, →0 as |d| grows past sigma. */
export function gauss(d: number, sigma: number): number {
  return Math.exp(-(d * d) / (2 * sigma * sigma))
}

/** Device-pixel ratio, capped at 2 to keep large canvases affordable. */
export function getDpr(): number {
  if (typeof window === 'undefined') return 1
  return Math.min(window.devicePixelRatio || 1, 2)
}
