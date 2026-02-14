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
