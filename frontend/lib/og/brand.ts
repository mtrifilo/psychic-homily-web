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
 * 2. Satori cannot parse WOFF2 or variable fonts, so the `.ttf` files next to
 *    this module are static Latin-subset instances of the site's faces:
 *    Schibsted Grotesk sets every body and headline run, and Golos Text sets
 *    the data runs (dates, the state, counts, the eyebrow, the wordmark).
 *
 * Regenerating the font files (only needed if a face is ever replaced), with
 * `fonttools` and `brotli` installed, run from `frontend/`. Both families are
 * OFL. Pin the full commit: `main` is mutable, and these bytes are parsed by a
 * wasm renderer on a public route, so they must be diffable. Verify each
 * download against the checksum in `lib/og/fonts/OFL.txt` before instancing.
 * `-g` stops curl reading `[wght]` as a URL range.
 *
 *     curl -gsSL -o '/tmp/SchibstedGrotesk[wght].ttf' \
 *       https://raw.githubusercontent.com/google/fonts/2fe1e8c6b137fb13041a7cfca6fdcd8a910ea4dc/ofl/schibstedgrotesk/SchibstedGrotesk[wght].ttf
 *     curl -gsSL -o '/tmp/GolosText[wght].ttf' \
 *       https://raw.githubusercontent.com/google/fonts/2fe1e8c6b137fb13041a7cfca6fdcd8a910ea4dc/ofl/golostext/GolosText[wght].ttf
 *     shasum -a 256 '/tmp/SchibstedGrotesk[wght].ttf' '/tmp/GolosText[wght].ttf'
 *     # Schibsted Grotesk: pin the weight axis at 700 and at 400, and rescale
 *     # the 2048-unit em to 1000 so the advances in textFit.ts are the font's
 *     # own integers rather than rounded ratios.
 *     python -c "from fontTools.ttLib import TTFont; from fontTools.varLib import instancer; \
 *       from fontTools.ttLib.scaleUpem import scale_upem; \
 *       [(f := instancer.instantiateVariableFont(TTFont('/tmp/SchibstedGrotesk[wght].ttf'), \
 *         {'wght': w}, updateFontNames=True), scale_upem(f, 1000), \
 *         f.save(f'/tmp/SchibstedGrotesk-{n}.ttf')) for w, n in ((700, 'Bold'), (400, 'Regular'))]"
 *     # Golos Text: pin wght 400, then FREEZE `tnum` into the cmap. Satori
 *     # applies only `kern`, `liga` and `rlig`, never `tnum`, so the data runs
 *     # get Golos's `tnum` figures (580 to 620 units wide, near-tabular) only
 *     # if they are the default glyphs.
 *     python -c "from fontTools.ttLib import TTFont; from fontTools.varLib import instancer; \
 *       f=instancer.instantiateVariableFont(TTFont('/tmp/GolosText[wght].ttf'), \
 *         {'wght':400}, updateFontNames=True); g=f['GSUB'].table; m={}; \
 *       [m.update(s.mapping) for r in g.FeatureList.FeatureRecord if r.FeatureTag=='tnum' \
 *         for i in r.Feature.LookupListIndex for s in g.LookupList.Lookup[i].SubTable]; \
 *       [t.cmap.update({u: m[n] for u, n in t.cmap.items() if n in m}) for t in f['cmap'].tables]; \
 *       f.save('/tmp/GolosText-Regular.ttf')"
 *     # Subset each to the Latin coverage the cards need. `tnum` is not kept:
 *     # it is frozen above where it matters, and Satori would never apply it.
 *     # <features> is "kern,liga,calt" for Schibsted Grotesk and "liga,calt"
 *     # for Golos Text; see KERNING below.
 *     pyftsubset /tmp/<face>.ttf --output-file=lib/og/fonts/<face>.ttf \
 *       --unicodes="U+0020-007E,U+00A0-00FF,U+0100-017F,U+2010-2027,U+2032-2033,U+20AC,U+2122" \
 *       --layout-features="<features>" --no-hinting --desubroutinize \
 *       --name-IDs='*' --name-legacy --name-languages='*'
 *
 * The `--name-*` flags keep the OFL notice in the bytes; see the fallback
 * recipe below for why that is not cosmetic. Each run stamps a new
 * `head.modified`, so a rebuild never matches the shipped file's checksum:
 * compare the tables (`ttx`), not the hashes.
 *
 * Golos Text 2.004 draws `Ğ ğ Ŭ` without their breve (google/fonts issue
 * 11219). The defect is upstream, and the cards draw the face as shipped
 * rather than patching the glyphs.
 *
 * KERNING: Satori sizes each word's box without kerning but draws its glyphs
 * with it, so a word with a tightening pair inside it (`Tuesday,`, `You!`) is
 * followed by a visibly wider space. Golos Text therefore ships WITHOUT
 * `kern`: the data runs (every card's date or range, the counts) would carry
 * that gap and gain nothing from kerning at their size. Schibsted Grotesk
 * keeps `kern`, because its pairs shape the display-size headlines; the wider
 * space after such a word in body and headline runs is accepted there.
 *
 * WEIGHTS: Schibsted Grotesk ships at 700 and 400 only. A 500 instance costs
 * every OG function in this family another ~23.8 KB brotli, and the largest of
 * them has no room for it (see the budget below), so the two runs the design
 * sets in Medium (the show card's venue line and the weekly card's count line)
 * request 400, and the venue line is measured as 400. A visible but deliberate degradation:
 * those two lines draw lighter than the design sets them. Satori resolves an
 * absent 500 to 400 anyway; asking for 400 keeps the requested, measured and
 * drawn face the same.
 *
 * Re-emit the metrics in `textFit.ts` whenever these files change.
 *
 * Kept free of runtime dependencies so the layout constants derived from it
 * stay cheap to import from a unit test.
 */

/*
 * ---------------------------------------------------------------------------
 * WHAT HAPPENS TO TEXT OUTSIDE THE SUBSET  (read this before re-subsetting)
 * ---------------------------------------------------------------------------
 *
 * Satori resolves each grapheme against EVERY face in the `fonts` array below —
 * the requested family first, then every other family, in registration order.
 * Only when no supplied face has the glyph does it call its asset loader, and
 * that loader fetches a typeface from **Google Fonts at render time**:
 *
 *     https://fonts.googleapis.com/css2?family=${font}&text=${encodeURIComponent(text)}
 *
 * These are public, unauthenticated routes, so that request means a card blocks
 * on a third party before responding, parses remote bytes as a typeface inside
 * the render, and — the part that actually matters — puts DB-derived text
 * (a venue name, a city, an artist) into a URL query string sent to Google.
 *
 * The supplied `fonts` array is the ONLY lever on this. `next/og` hardcodes
 * `loadAdditionalAsset` when it constructs Satori and `ImageResponseOptions`
 * has no field to override it, so the loader cannot be replaced or disabled
 * without patching next's vendored bundle. Widening coverage is therefore
 * widening this array.
 *
 * DECISION: cover the LIVING languages a worldwide scene table will realistically
 * emit, and accept that the long tail still leaves the page.
 *
 * Schibsted Grotesk is a Latin-only TYPEFACE, not merely a Latin-only subset:
 * its full charset carries no Cyrillic at all and eight stray Greek
 * codepoints, so the body face CANNOT be widened past Latin. Golos Text does
 * draw Cyrillic, but its subset here is Latin like the body face's, so every
 * script past Latin resolves through one separate fallback face, which is what
 * `NotoSans-Regular.ttf` is. It is never named in a `fontFamily`; it exists
 * purely to be found by the cross-family search described above.
 *
 * ---------------------------------------------------------------------------
 * THE BINDING CONSTRAINT IS A 1 MiB VERCEL EDGE FUNCTION LIMIT — NOT TASTE
 * ---------------------------------------------------------------------------
 *
 * Every OG route in this family is an Edge Function (six of them, each carrying
 * its own copy of these fonts), and Vercel caps a deployed Edge Function at
 * 1 MiB on this plan. The build succeeds locally either way; it is the DEPLOY
 * that is rejected, so nothing in `next build` will warn you.
 *
 * Measured from `vercel build`'s `.vercel/output` (the metric that matches
 * Vercel's reported number is raw `index.js` + brotli-compressed assets) on
 * the largest function, `shows/[slug]/opengraph-image`:
 *
 *     index.js                                     946 KB   raw, counted UNCOMPRESSED
 *     brand faces (Schibsted 700 + 400, Golos)      59 KB   brotli
 *     the coverage fallback below (29 KB raw)       13 KB   brotli
 *     @vercel/og's own fallback face                15 KB   brotli
 *     ---------------------------------------------------
 *     shipped                                       0.986 MiB  (1,033,936 B)
 *
 * So the largest route sits at ~98.6% of the limit: about 14 KiB of brotli
 * headroom for everything. Measured against that, a Schibsted Grotesk 500
 * instance adds 23.8 KB (1.009 MiB) and a Hanken Grotesk 700 instance for
 * headlines adds 15.1 KB (1,049,046 B, 470 B over); neither fits. A previous revision of the
 * fallback below shipped its coverage at two weights across wider blocks
 * (242 KB raw) and was REJECTED at deploy: 1.067 MiB.
 *
 * That is why the subset below is cut to living-language ranges rather than whole
 * Unicode blocks, and why only ONE weight ships. Before widening anything here,
 * re-measure, and be aware that most of the 946 KB is next/og's satori + resvg
 * glue plus the globally-injected Sentry/OpenTelemetry edge SDK (about half the
 * module graph), neither of which this file controls.
 *
 * Measured against production data (4,941 artists, 237 venues, 4,119 upcoming
 * shows, 26 scenes = 25,333 card-bound strings): 10 strings — 0.039% — contained
 * anything outside the Latin subset, and NONE of it was a non-Latin script. Every
 * scene, venue and show-title string was already fully covered. The whole of the
 * miss was three decorative symbols in band names (‧ U+2027, ★ U+2605, ♱ U+2671)
 * and five C1 control bytes from double-decoded UTF-8 upstream — the latter a
 * data-quality bug, not a script this card should be designing for.
 *
 * So the Greek and Cyrillic coverage below is deliberate HEADROOM for the
 * worldwide scene table, not a response to rows that exist today. Of the three
 * symbols, only ‧ falls in a block this subset picks up (General Punctuation);
 * ★ and ♱ remain uncovered, and are the one gap today's data actually hits.
 *
 * COVERED (no outbound request) — verified glyph-by-glyph in the test:
 *   Latin, Latin-1, Latin Ext-A          Schibsted Grotesk / Golos Text
 *   Greek, modern monotonic              fallback  Αθήνα, Θεσσαλονίκη
 *   Cyrillic, all living Slavic          fallback  Москва, Київ, Београд,
 *                                                  София, Скопје, Мінск
 *   Cyrillic beyond the base block       fallback  Ґ ґ (Ukrainian), Ә ә, Ө ө
 *   Latin Ext-B, targeted                fallback  Ș ș Ț ț (Romanian),
 *                                                  Ə ə (Azerbaijani),
 *                                                  Ǆ-ǌ (Croatian digraphs), ƒ
 *   Typographic punctuation, №           fallback
 *
 * NOT COVERED — these STILL reach Google / jsDelivr at render time:
 *   CJK, Hangul, Arabic, Hebrew, Devanagari, Thai and every other script.
 *     A usable CJK face is megabytes; it cannot ride in an edge bundle at all,
 *     let alone in the ~14 KiB this route family has spare. Closing this needs a
 *     different architecture (satori + resvg directly, or patching next's
 *     vendored loader), which is deliberately deferred.
 *   Vietnamese (Latin Ext Additional, U+1E00-1EFF).
 *     Genuinely a Latin script and a real gap — `Đà Nẵng` renders `Đ` and `à`
 *     from the brand face but fetches `ẵ`. It is excluded only because the block
 *     costs 27 KB raw on its own, which does not fit under the limit above. This
 *     is the cheapest thing to add back if the limit ever rises.
 *   Decorative symbols — stars, crosses, dingbats, arrows, card suits.
 *     Noto Sans genuinely has none of these (verified against the shipped
 *     bytes: zero glyphs in Misc Symbols, Dingbats or Arrows), so asking for
 *     those ranges here would be a no-op. This is the one gap the production
 *     sample actually exercised — ★ U+2605 in "V★Silver" and ♱ U+2671 — but the
 *     category is unbounded, so covering the two seen today would buy little.
 *     Closing it means a symbols face (Noto Sans Symbols 2), not a wider subset.
 *   Emoji — ALWAYS, and unconditionally.
 *     Every `emoji` option `@vercel/og` accepts names a CDN (twemoji, openmoji,
 *     blobmoji, noto, fluent — all jsdelivr). There is no local emoji option, so
 *     an emoji in a band name is an outbound request no subset can prevent.
 *
 * WEIGHT: only 400 ships. Satori picks the nearest weight within a family, so a
 * non-Latin headline — which the card sets at 700 — is drawn at regular weight.
 * A visible but deliberate degradation: the second weight cost 121 KB raw and
 * the budget above has no room for it, and this face covers 0% of current
 * production rows, so it buys appearance on text that does not exist yet.
 *
 * Out-of-subset text is NOT stripped or transliterated: mangling a real place
 * name to protect the render is a worse failure than the fetch it avoids.
 * `textFit.ts` over-estimates anything it cannot measure so such a card CLIPS
 * rather than bleeding off the canvas.
 *
 * Regenerating the fallback (`fonttools` + `brotli` installed) — Noto Sans is
 * OFL; the ref is pinned because `main` is mutable and these bytes are parsed by
 * a wasm renderer on a public route:
 *
 *     curl -gsSLO https://raw.githubusercontent.com/google/fonts/2984c575fdce412ee02b2baaba67672b9a9434d8/ofl/notosans/'NotoSans[wdth,wght].ttf'
 *     shasum -a 256 'NotoSans[wdth,wght].ttf'
 *     # bfb7bb691513f12e734dc346c03a03f784912432d7e3fa8e56efcf906fe86b3d
 *     # Pin the variable axes, then subset.
 *     python -c "from fontTools.ttLib import TTFont; from fontTools.varLib import instancer; \
 *       f=instancer.instantiateVariableFont(TTFont('NotoSans[wdth,wght].ttf'), \
 *         {'wght':400,'wdth':100}); f.save('/tmp/NotoSans-Regular.ttf')"
 *     pyftsubset /tmp/NotoSans-Regular.ttf --output-file=lib/og/fonts/NotoSans-Regular.ttf \
 *       --unicodes="U+0384-038A,U+038C,U+038E-03A1,U+03A3-03CE,U+0400-045F,U+0490-0491,U+04D8-04D9,U+04E8-04E9,U+018F,U+0192,U+01C4-01CC,U+0218-021B,U+0259,U+2010-2015,U+2018-201A,U+201C-201E,U+2020-2022,U+2026-2027,U+2030,U+2039-203A,U+2116" \
 *       --layout-features="kern,liga,calt" --no-hinting --desubroutinize \
 *       --name-IDs='*' --name-legacy --name-languages='*'
 *
 * The `--name-*` flags are not cosmetic: pyftsubset keeps only name IDs 0-6 by
 * default, and the OFL requires the copyright and licence notice to travel with
 * the bytes. Dropping them would strip the notice out of a redistributed font.
 *
 * The ranges are enumerated per living language rather than as whole blocks, and
 * that precision is what buys the size. Whole-block equivalents cost roughly:
 * Greek U+0370-03FF 15 KB, Cyrillic U+0400-04FF+Supplement 38 KB, Latin Ext-B
 * U+0180-024F 25 KB, Latin Ext Additional 27 KB, against about 14 KiB of
 * brotli headroom for the largest route. Hence the enumeration.
 *
 * The fallback deliberately carries NO Latin: the brand faces are searched
 * first and always win there, so Latin glyphs in this file would be dead bytes
 * on a route that has no bytes to spare.
 *
 * `textFit.fonts.test.ts` asserts this coverage against the shipped bytes, so a
 * re-subset that silently drops a script fails the suite instead of quietly
 * reintroducing the outbound fetch.
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
  /**
   * `background` at zero alpha — the far end of any fade that reveals what is
   * painted under it.
   *
   * Sited here, next to the colour it must equal, because it IS that colour: a
   * card that spelled the same rgb a second way would keep the old hue when the
   * token moves, and a wrong gradient is invisible in review.
   *
   * An explicit `rgba` rather than the keyword `transparent`, which several
   * renderers interpolate through `rgba(0,0,0,0)` — greying the middle of a
   * fade from a warm near-black.
   */
  backgroundClear: 'rgba(13, 8, 5, 0)',
  foreground: '#eee7d9',
  primary: '#e89960',
  mutedForeground: '#9c8c7c',
  destructive: '#f14d4c',
  /**
   * White, not `--destructive-foreground` (`#0d0805`).
   *
   * A deliberate departure: the token pairs dark-on-red for in-app badges,
   * but a share card is read at a quarter size on someone else's feed, where
   * white-on-red holds far more contrast. Written down so it reads as a
   * decision rather than the drift that produced `#b8a99a`.
   */
  destructiveForeground: '#ffffff',
} as const

/** Family names registered with Satori; use these in `fontFamily`. */
export const OG_FONT_FAMILY = {
  sans: 'Schibsted Grotesk',
  data: 'Golos Text',
} as const

/**
 * Family name of the script-coverage fallback.
 *
 * Deliberately NOT part of `OG_FONT_FAMILY`: nothing should ever set this as a
 * `fontFamily`. It is registered only so Satori's cross-family glyph search can
 * find it, which is what keeps a non-Latin name from becoming a Google Fonts
 * request mid-render.
 *
 * Setting it as a `fontFamily` would be actively wrong rather than merely
 * redundant — this face carries no Latin at all, so a card that named it would
 * resolve its ordinary text through the fallback chain instead of directly, and
 * every layout budget in `textFit.ts` is keyed to a `SansFace` that this is not.
 */
const OG_FALLBACK_FAMILY = 'PH Fallback'

/**
 * Satori's font descriptor. Declared locally rather than imported because
 * `next/og` does not re-export it.
 */
export interface OgFont {
  name: string
  data: ArrayBuffer
  weight: 400 | 700
  style: 'normal'
}

/**
 * Cached across requests in the same isolate, and deliberately cached as ONE
 * ARRAY rather than as separate buffers.
 *
 * Satori memoizes its parsed font loader in a `WeakMap` keyed on the identity
 * of the array it is handed. Building a fresh array per render therefore misses
 * that cache every single time and re-parses every face, measured at
 * roughly 8–15% of render time, plus ~150KB of buffer churn per request. The
 * same trick is what `@vercel/og` itself uses for its fallback font.
 */
let fontsPromise: Promise<BrandFontSet> | undefined

/**
 * The loaded faces, plus whether script coverage is complete.
 *
 * The two groups fail differently and must not be conflated. Losing a BRAND
 * face is a layout problem — every fit budget on a card is computed from
 * the brand faces' metrics, so the card may overrun. Losing the FALLBACK is a privacy
 * and consistency problem — the card still lays out correctly, but any
 * non-Latin text silently goes back to being fetched from Google mid-render,
 * which is the exact behaviour this module exists to prevent.
 */
export interface BrandFontSet {
  fonts: OgFont[]
  /** True when the script-coverage fallback could not be loaded. */
  fallbackMissing: boolean
}

/**
 * Load the three brand faces and the script-coverage fallback.
 *
 * `new URL(..., import.meta.url)` is what makes the bundler treat the `.ttf`
 * files as route assets, so these must stay static relative literals — a
 * computed path would silently fail to bundle and the card would fall back to
 * Satori's default font at runtime.
 */
export function loadBrandFonts(): Promise<BrandFontSet> {
  // A rejected promise must not be retained: pinning it would strand every
  // later request in this isolate on the off-brand fallback card, which is the
  // opposite of what the failure path is for.
  fontsPromise ??= fetchBrandFonts().catch(error => {
    fontsPromise = undefined
    throw error
  })
  return fontsPromise
}

/**
 * Fetch one font asset, failing loudly on anything that is not a real font.
 *
 * `arrayBuffer()` resolves perfectly happily on a 404 or 500 body, which would
 * hand Satori an error page as a typeface — and because the loader memoizes on
 * success, those garbage bytes would be pinned for the life of the isolate and
 * every later render would fail inside the PNG stream, after the 200 response
 * had already been sent. Better to reject here so the fallback path can run.
 */
async function fetchFont(url: URL): Promise<ArrayBuffer> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new Error(`OG font asset ${url.pathname} responded ${res.status}`)
  }
  const data = await res.arrayBuffer()
  // Every TrueType/OpenType file opens with one of these sfnt version tags.
  const tag = new DataView(data).getUint32(0)
  if (tag !== 0x00010000 && tag !== 0x4f54544f && tag !== 0x74727565) {
    throw new Error(`OG font asset ${url.pathname} is not a font (tag ${tag.toString(16)})`)
  }
  return data
}

async function fetchBrandFonts(): Promise<BrandFontSet> {
  // The brand faces are REQUIRED: every fit budget on a card is derived from
  // their metrics, so a card without them is measured against the wrong numbers.
  // Losing one is worth failing the whole load for.
  const brand = Promise.all([
    fetchFont(new URL('./fonts/SchibstedGrotesk-Bold.ttf', import.meta.url)),
    fetchFont(new URL('./fonts/SchibstedGrotesk-Regular.ttf', import.meta.url)),
    fetchFont(new URL('./fonts/GolosText-Regular.ttf', import.meta.url)),
  ])

  // The fallback is ADDITIVE — it only ever adds glyph coverage — so it must not
  // be able to take the brand faces down with it. Sharing one `Promise.all`
  // would mean a bad fallback asset dropped every face and rendered every card,
  // Latin included, in Satori's default typeface: strictly worse than the
  // coverage gap it was trying to report.
  const fallback = fetchFont(
    new URL('./fonts/NotoSans-Regular.ttf', import.meta.url)
  ).catch(() => null)

  const [[bold, regular, data], fallbackFace] = await Promise.all([brand, fallback])

  // Order matters: Satori searches the requested family first and then every
  // other family in THIS order, so the brand faces must precede the fallback.
  // Nothing depends on it TODAY — the fallback carries no Latin, so there is no
  // glyph both could serve — but that is a property of the current subset, not
  // of the design. Anyone who widens the fallback to overlap Latin would
  // otherwise silently start drawing brand text in Noto Sans.
  const fonts: OgFont[] = [
    { name: OG_FONT_FAMILY.sans, data: bold, weight: 700, style: 'normal' },
    { name: OG_FONT_FAMILY.sans, data: regular, weight: 400, style: 'normal' },
    { name: OG_FONT_FAMILY.data, data, weight: 400, style: 'normal' },
  ]

  // Registered at 400 only. Satori resolves weight WITHIN a family, so this one
  // face answers every request against it — including the 700 the card sets on
  // its headline, which is drawn at regular weight. See the WEIGHT note above.
  if (fallbackFace) {
    fonts.push({ name: OG_FALLBACK_FAMILY, data: fallbackFace, weight: 400, style: 'normal' })
  }

  return { fonts, fallbackMissing: fallbackFace === null }
}
