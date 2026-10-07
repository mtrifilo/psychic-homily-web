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

## `/atlas` map: who gets it, and the phone perf gate

The Atlas map is MapLibre GL (`maplibre-gl` 6.0.0), lazy-loaded via
`dynamic(() => import('./GlobeCanvas'), { ssr: false })` in
`features/scenes/components/AtlasGlobe.tsx`, so the canvas module loads only
on `/atlas` and the map never server-renders. `preloadAtlasMap`
(`atlasMapPreload.ts`) starts the same import while the scenes and the
visitor's geo resolve.

### Who gets the map

Every width gets the map, phones included. `atlasRendersSceneList`
(`features/scenes/atlasViewport.ts`) is the whole fallback rule. Where it
holds, `AtlasSceneList` (every scene, liveliest first, each row expanding in
place) stands in for the map:

1. **No WebGL2, at any width.** `atlasSupportsWebGL2` probes a 1x1 canvas once
   per page load; MapLibre asks only for `webgl2`.
2. **Reduced motion on a narrow pane.** The visitor prefers reduced motion and
   the Atlas pane is narrower than `ATLAS_REDUCED_MOTION_LIST_BELOW_PX`
   (640px). Wider panes keep the map, which honours the preference itself: no
   pulse rings, and camera moves cut instead of fly.
3. **A failed map.** GlobeCanvas sits inside a `GraphSectionErrorBoundary`
   (Sentry tag `atlas-map`). When MapLibre is refused a WebGL2 context after
   the probe passed, GlobeCanvas throws `AtlasMapContextError` and the list
   latches for the rest of the page load (`markAtlasMapFailed`). Any other
   error React sees from the canvas falls back for that mount: a throwing
   effect is tried again on the next mount, but a failed canvas chunk is not
   refetched (`React.lazy` keeps the rejected import), so every later mount
   in that page load gets the list too, until a reload. Errors thrown from
   MapLibre's own callbacks (animation frames, map events) are not React
   errors and are not caught.

### The light look on compact viewports

Below Tailwind's `lg` (64rem, `ATLAS_COMPACT_VIEWPORT_QUERY`), the globe draws
the light look (`features/scenes/basemap/globeSurface.ts`) instead of the NASA
GIBS night-earth raster: a flat ocean, Natural Earth 1:110m land, 1:110m
country and 1:50m state or province boundary lines, and Natural Earth
populated-place labels drawn as DOM markers (`globePlaces.ts`). Each is one
small same-origin GeoJSON file under `public/atlas/`. The hidden raster layer
leaves its source unused, so MapLibre fetches no GIBS tile and lists no NASA
credit. Viewports at `lg` and wider keep the raster.

### Phone perf gate

The owner's gate for the phone entry, revised on 2026-10-04 from the original
2.5 s:

| measure | gate | target |
| --- | --- | --- |
| first rendered map | at most 3.5 s | 2.5 s (the app-shell diet, PSY-2171) |
| entry bytes | at most 1.5 MiB | |

Profile: Playwright's `iPhone 13` at 390x844 (DPR 3), 4x CPU throttling on the
main thread only, Fast 4G, cold cache, medians of cold runs. GPU and worker
threads are not throttled, so first rendered map is a lower bound for a real
phone.

`scripts/atlas-perf.mjs` measures it: `bun run perf:atlas <url> --runs 3`
against a production build (a Vercel preview, or `next build && next start`;
never `next dev`). Its header documents the profile, the readiness gate and
what counts as entry bytes. It sets up none of the scene-list conditions and
stops with exit code 2 if the page shows the list, and it fails a compact run
on any raster request. The pass or fail line it prints follows the script's
own `BUDGET` constant; read the medians against the gate above.

Measured at the flip (PR #2189, Vercel preview of `509ea6109`, stage data,
three sessions of three cold headed runs): first rendered map nine-run median
3.38 s, worst session median 3,502 ms (2 ms over the gate); entry 1.48 MiB
(1,511 KiB); no raster request. City view (Chicago, z12.5) added 280 KiB and
was ready in 0.59 s. Desktop (1440x900) is not budgeted.

No Lighthouse budget covers `/atlas`: the Lighthouse CI job
(`.github/workflows/lighthouse-explore.yml`, settings in `lighthouserc.json`)
collects `/explore` only.

---

## Visible does not imply interactive (PSY-1610 / PSY-1615)

**Anything server-rendered is clickable before React wires it up, and a click in
that window is discarded, not queued.** React 19 does not replay it. There is no
error and no console warning — the control simply does not respond.

Measured on a production build, median of 3, from FCP to the moment that node
became interactive:

| condition | dead window |
| --- | --- |
| M-series laptop, loopback | ~260 ms |
| 4x CPU throttle | ~505 ms |
| 6x CPU + 1.6 Mbps / 150 ms RTT | ~4.6 s |
| 20x CPU + same network | ~6.7 s |

Proof, not inference: React stamps a `__reactProps$…` key on a host node exactly
when it hydrates that node. Correlating that with a capture-phase click listener
gave **39 pre-hydration clicks → 0 effects, 42 post-hydration → 42 effects**, no
overlap.

This is a property of client-side hydration, **not** of authentication —
anonymous surfaces have it too. Page-body controls are consistently the *last*
things to wake up, later than the TopBar cluster.

What this means when writing SSR-rendered UI:

- **Navigation must be a real `<a href>` (or `next/link`), never an `onClick`
  router push.** Anchors are handled by the browser, so they work throughout the
  window for free. This is the single most valuable rule here — it is why the
  `+ Submit` link never needed fixing.
- **Interactive controls that ship in server HTML should opt into click
  replay**: spread `{...replayOnHydrate}` onto the control. `BracketLink`
  already carries it, so every bracket control inherits it for free. See
  `lib/hydration/clickReplay.ts` for the mechanism and its exactly-once
  guarantees. Limits: **a captured click expires after 10 s**
  (`MAX_REPLAY_AGE_MS`) and is then dropped silently, and only clicks are
  captured — keyboard activation of a Radix trigger goes through `onKeyDown`
  and is still dropped.
- **Do not "fix" this by gating rendering on hydration** — `suppressHydrationWarning`,
  `typeof window` render branches, and effect-gated rendering all hide the
  divergence instead of resolving it, and disabling a control for up to 6.7 s is
  a worse regression than the dropped click.

The proof harness is committed at `e2e-hydration/` so the next person does not
have to build it a third time. Run it with `bun run test:e2e:hydration` (it
needs a production build and its own stack — see the header of
`e2e-hydration/prehydration-replay.spec.ts`). It is the ONLY guard against the
double-fire regression: the unit suite cannot catch a move from the ref
callback to a passive effect, because jsdom does not reproduce that gap.

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
