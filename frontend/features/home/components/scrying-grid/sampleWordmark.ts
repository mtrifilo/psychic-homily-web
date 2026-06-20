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
  weight?: number
  /** Sampling step in device px — smaller = denser field. */
  gapDev?: number
  alphaThreshold?: number
  /** Scale the fit box (<1 = more margin around the wordmark inside the canvas). */
  padScale?: number
}

/** Sample the wordmark into a list of lit points (device-px coords). */
export function sampleWordmark(widthDev: number, heightDev: number, opts: SampleOpts): SampledPoint[] {
  const weight = opts.weight ?? 700
  const canvas = document.createElement('canvas')
  canvas.width = Math.max(1, Math.round(widthDev))
  canvas.height = Math.max(1, Math.round(heightDev))
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return []

  const pad = opts.padScale ?? 1
  const size = fitFontSize(ctx, opts.lines, canvas.width * 0.86 * pad, canvas.height * 0.78 * pad, opts.fontFamily, weight)
  ctx.font = `${weight} ${size}px ${opts.fontFamily}`
  ctx.fillStyle = '#ffffff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const lineHeight = size * LINE_GAP_FACTOR
  const n = opts.lines.length
  for (let i = 0; i < n; i++) {
    const y = canvas.height / 2 - ((n - 1) / 2) * lineHeight + i * lineHeight
    ctx.fillText(opts.lines[i], canvas.width / 2, y)
  }

  const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height)
  const gap = Math.max(2, Math.round(opts.gapDev ?? 7))
  const threshold = opts.alphaThreshold ?? 140
  const points: SampledPoint[] = []
  for (let y = 0; y < canvas.height; y += gap) {
    for (let x = 0; x < canvas.width; x += gap) {
      if (data[(y * canvas.width + x) * 4 + 3] > threshold) points.push({ x, y })
    }
  }
  return points
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
