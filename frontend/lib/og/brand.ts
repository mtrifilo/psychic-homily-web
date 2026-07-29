/**
 * Shared brand surface for OG share cards.
 *
 * Share cards render on someone else's feed, so they are brand surfaces rather
 * than themed UI: DARK ONLY, one committed look, no light variant (PSY-1576
 * decision 4).
 *
 * Two things force this module to exist instead of reusing the app's styling:
 *
 * 1. `next/og` renders through Satori in an isolated context that cannot read
 *    the app's CSS, so the DS Dark token VALUES have to be inlined here. They
 *    are copied from the `.dark` block in `app/globals.css` and must be kept in
 *    step with it — a drift shows up as a card that no longer matches the site.
 * 2. Satori cannot parse WOFF2, which is the only format `app/fonts/` ships.
 *    The `.ttf` files next to this module are Latin-subset conversions of the
 *    exact same faces the site loads via `next/font`.
 *
 * Regenerating the font files (only needed if a face is ever replaced) — with
 * `fonttools` and `brotli` installed:
 *
 *     # Satoshi: unwrap the WOFF2 the site already ships, then subset
 *     python -c "from fontTools.ttLib import TTFont; \
 *       f=TTFont('app/fonts/Satoshi-Bold.woff2'); f.flavor=None; \
 *       f.save('/tmp/Satoshi-Bold.ttf')"
 *     # Space Mono: OFL, from the upstream Google Fonts repo
 *     curl -sSLO https://raw.githubusercontent.com/google/fonts/main/ofl/spacemono/SpaceMono-Regular.ttf
 *     # Subset each to the Latin coverage the cards need
 *     pyftsubset <in>.ttf --output-file=lib/og/fonts/<out>.ttf \
 *       --unicodes="U+0020-007E,U+00A0-00FF,U+0100-017F,U+2010-2027,U+2032-2033,U+20AC,U+2122" \
 *       --layout-features="kern,liga,calt" --no-hinting --desubroutinize
 *
 * Re-emit the metrics in `textFit.ts` whenever these files change.
 *
 * Kept free of runtime dependencies so the layout constants derived from it
 * stay cheap to import from a unit test.
 */

/** Every card in the family is a standard 1200×630 OG image. */
export const OG_SIZE = { width: 1200, height: 630 } as const

export const OG_CONTENT_TYPE = 'image/png'

/**
 * DS Dark tokens, inlined.
 *
 * Mirrors `--background` / `--foreground` / `--primary` / `--muted-foreground`
 * from the `.dark` block of `app/globals.css`.
 */
export const OG_COLORS = {
  background: '#0d0805',
  foreground: '#eee7d9',
  primary: '#e89960',
  mutedForeground: '#9c8c7c',
} as const

/** Family names registered with Satori; use these in `fontFamily`. */
export const OG_FONT_FAMILY = {
  sans: 'Satoshi',
  mono: 'Space Mono',
} as const

/**
 * Satori's font descriptor. Declared locally rather than imported because
 * `next/og` does not re-export it.
 */
export interface OgFont {
  name: string
  data: ArrayBuffer
  weight: 400 | 500 | 700
  style: 'normal'
}

/**
 * Cached across requests in the same isolate, and deliberately cached as ONE
 * ARRAY rather than as four buffers.
 *
 * Satori memoizes its parsed font loader in a `WeakMap` keyed on the identity
 * of the array it is handed. Building a fresh array per render therefore misses
 * that cache every single time and re-parses all four faces — measured at
 * roughly 8–15% of render time, plus ~150KB of buffer churn per request. The
 * same trick is what `@vercel/og` itself uses for its fallback font.
 */
let fontsPromise: Promise<OgFont[]> | undefined

/**
 * Load the four faces every card in the family uses.
 *
 * `new URL(..., import.meta.url)` is what makes the bundler treat the `.ttf`
 * files as route assets, so these must stay static relative literals — a
 * computed path would silently fail to bundle and the card would fall back to
 * Satori's default font at runtime.
 */
export function loadBrandFonts(): Promise<OgFont[]> {
  // A rejected promise must not be retained: pinning it would strand every
  // later request in this isolate on the off-brand fallback card, which is the
  // opposite of what the failure path is for.
  fontsPromise ??= fetchBrandFonts().catch(error => {
    fontsPromise = undefined
    throw error
  })
  return fontsPromise
}

async function fetchBrandFonts(): Promise<OgFont[]> {
  const [bold, medium, regular, mono] = await Promise.all([
    fetch(new URL('./fonts/Satoshi-Bold.ttf', import.meta.url)).then(r => r.arrayBuffer()),
    fetch(new URL('./fonts/Satoshi-Medium.ttf', import.meta.url)).then(r =>
      r.arrayBuffer()
    ),
    fetch(new URL('./fonts/Satoshi-Regular.ttf', import.meta.url)).then(r =>
      r.arrayBuffer()
    ),
    fetch(new URL('./fonts/SpaceMono-Regular.ttf', import.meta.url)).then(r =>
      r.arrayBuffer()
    ),
  ])

  return [
    { name: OG_FONT_FAMILY.sans, data: bold, weight: 700, style: 'normal' },
    { name: OG_FONT_FAMILY.sans, data: medium, weight: 500, style: 'normal' },
    { name: OG_FONT_FAMILY.sans, data: regular, weight: 400, style: 'normal' },
    { name: OG_FONT_FAMILY.mono, data: mono, weight: 400, style: 'normal' },
  ]
}
