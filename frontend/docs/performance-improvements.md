# Performance Improvement Opportunities

Quick wins that don't require major refactors. Based on Vercel's React Best Practices (January 2026).

## Completed Optimizations

- [x] `optimizePackageImports` for lucide-react, radix-ui, date-fns, tanstack-query
- [x] Dynamic imports for admin components (ShowImportPanel, VenueEditsPage)
- [x] React.memo for pure display components (ImportPreview, RejectedShowCard)
- [x] Viewport export with theme-color for light/dark
- [x] DNS prefetch & preconnect for API, Spotify, Bandcamp
- [x] Route-level loading states (shows, venues, admin, collection)
- [x] Logo image `priority` for faster LCP
- [x] SVG logo optimized with SVGO (467KB → 368KB)
- [x] Link prefetch hints (disabled for admin, submissions, substack)
- [x] Bundle analyzer (`bun run analyze`)
- [x] Static generation — `/blog`, `/dj-sets`, `/venues` already statically rendered (filesystem content + client-side fetching)
- [x] Parallel data fetching — `discover-music/route.ts` already uses `Promise.allSettled`; `oembed/route.ts` is single-fetch
- [x] Detail page server fetches — `shows/[slug]`, `venues/[slug]`, `artists/[slug]` each make a single fetch (deduplicated by Next.js between `generateMetadata` and page component)

---

## `/atlas` globe — perf budget (PSY-1222)

The `/atlas` globe (PSY-1213) renders `react-globe.gl` + `three.js`, lazy-loaded
via `dynamic(() => import('./GlobeCanvas'), { ssr: false })` in
`features/scenes/components/AtlasGlobe.tsx` behind a lightweight shell — so the
heavy chunk streams in **after** the surrounding UI is interactive, and the
globe itself never server-renders.

This is a documented **target budget, NOT a CI gate** (per the 2026-06-29 scope
decision on PSY-1222): a regression is a signal to investigate, not a merge
blocker. The existing `/explore` Lighthouse gate (`lighthouserc.json`) is itself
`warn`-only for the same reason — CI-runner CPU contention swings TTI by more
than the whole budget, so an `error` assertion would block merges on noise.

### Measured (local prod build, 3-run median)

`cd frontend && bun run build && bun run start`, then Lighthouse. The **mobile**
row uses the same settings as the `/explore` gate (`lighthouserc.json`: Moto G4,
slow-4G — 150ms RTT / 1638 Kbps / 4× CPU, performance-only). The **desktop** row
uses the Lighthouse `desktop` preset — the globe is desktop-primary; on mobile
`/atlas` shows the scene list (PSY-1311), not the globe.

| form factor | LCP | TTI | TBT | Speed Index | CLS | LCP element |
| --- | --- | --- | --- | --- | --- | --- |
| Mobile (slow-4G, 4× CPU) | 0.73s | 0.84s | 110ms | 5.8s | 0.000 | scene-list message (`<p>`) |
| Desktop (`desktop` preset) | 1.19s | 1.19s | 0ms | 0.75s | 0.001 | search bar (`AtlasSearch`) |

On **both** form factors the LCP element is **surrounding UI** — the search bar
on desktop, the "browse the scenes below" message on mobile — **not the globe
canvas**: the lazy chunk (largest observed script ~203 kB gz; the PSY-1211 spike
measured ~469 kB gz for the three.js bundle) loads but never becomes the largest
paint or blocks interactivity. The three.js cost surfaces only in **Speed
Index** under mobile throttle (~5.8s: the heavier `/atlas` payload / WebGL canvas
painting progressively behind the interactive UI), which is expected for a
WebGL-primary page and does not affect TTI/LCP.

### Target budget (informational — enforced by review, NOT by CI)

Anchored to the `/explore` budget; `/atlas` meets it with margin:

- **LCP < 2.0s** — measured 0.73s (mobile) / 1.19s (desktop)
- **TTI < 2.5s** — measured 0.84s (mobile) / 1.19s (desktop)
- **CLS < 0.1** — measured 0.000 mobile / 0.001 desktop (a fixed-height
  container, `h-[calc(100dvh-4rem)] min-h-[480px]`, pre-sizes the content area on
  both form factors; on desktop the `GlobeSkeleton` also reserves the canvas box)
- **Speed Index** — no hard target (WebGL-primary); ~5.8s mobile / 0.75s desktop
  today. Investigate if: the lazy chunk grows materially, the globe canvas
  becomes the LCP element (the shell stopped painting first), or TTI/LCP regress
  well past the values above.

Re-measure: build + serve as above, then
`node_modules/.bin/lhci collect --url=http://localhost:3000/atlas` (reads
`lighthouserc.json`) for the mobile numbers, or
`node_modules/.bin/lighthouse http://localhost:3000/atlas --preset=desktop` for
desktop. Measure against a local prod build (not a Vercel preview) to avoid the
`x-vercel-protection-bypass` CORS false-failure that inflates preview TTI.

---

## Measuring Results

### Lighthouse
```bash
# Run in production mode
bun run build && bun run start
# Then run Lighthouse in Chrome DevTools
```

### Web Vitals
Monitor these metrics:
- **LCP** (Largest Contentful Paint): < 2.5s
- **FID** (First Input Delay): < 100ms
- **CLS** (Cumulative Layout Shift): < 0.1
- **TTFB** (Time to First Byte): < 800ms

### Bundle Analysis
```bash
bun run analyze
```

---

## References

- [Vercel React Best Practices](https://vercel.com/blog/introducing-react-best-practices)
- [Next.js Performance](https://nextjs.org/docs/app/building-your-application/optimizing)
- [Web Vitals](https://web.dev/vitals/)
