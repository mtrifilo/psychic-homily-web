# Performance Improvement Opportunities

Quick wins that don't require major refactors. Based on Vercel's React Best Practices (January 2026).

## Completed Optimizations

- [x] `optimizePackageImports` for lucide-react, radix-ui, date-fns, tanstack-query
- [x] Dynamic imports for admin components (ShowImportPanel, VenueEditsPage)
- [x] React.memo for pure display components (ImportPreview, RejectedShowCard)

---

## Pending Opportunities

### 1. Metadata & Viewport Config

**Impact**: Better SEO, faster first paint
**Effort**: 5 minutes
**File**: `app/layout.tsx`

```tsx
import type { Metadata, Viewport } from 'next'

export const metadata: Metadata = {
  title: {
    default: 'Psychic Homily - Phoenix Music Shows',
    template: '%s | Psychic Homily',
  },
  description: 'Discover upcoming live music shows in Phoenix, AZ',
  metadataBase: new URL('https://psychichomily.com'),
  openGraph: {
    title: 'Psychic Homily',
    description: 'Discover upcoming live music shows in Phoenix, AZ',
    type: 'website',
  },
}

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: 'white' },
    { media: '(prefers-color-scheme: dark)', color: 'black' },
  ],
}
```

---

### 2. Prefetch Critical Routes

**Impact**: Instant navigation feel
**Effort**: 5 minutes
**File**: `app/nav.tsx`

Next.js prefetches `<Link>` by default in production, but explicitly marking critical routes ensures they're prioritized:

```tsx
// High-traffic routes - ensure prefetch
<Link href="/shows" prefetch={true}>Shows</Link>
<Link href="/venues" prefetch={true}>Venues</Link>

// Lower traffic routes - disable prefetch to save bandwidth
<Link href="/admin" prefetch={false}>Admin</Link>
```

---

### 3. Route Loading States

**Impact**: Perceived performance boost, no layout shift
**Effort**: 10 minutes

Create `loading.tsx` files for instant skeleton feedback:

**File**: `app/shows/loading.tsx`
```tsx
import { Loader2 } from 'lucide-react'

export default function Loading() {
  return (
    <div className="flex items-center justify-center min-h-[50vh]">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}
```

Create for:
- `app/shows/loading.tsx`
- `app/venues/loading.tsx`
- `app/admin/loading.tsx`
- `app/artists/[id]/loading.tsx`

---

### 4. DNS Prefetch & Preconnect

**Impact**: 100-300ms faster API calls
**Effort**: 2 minutes
**File**: `app/layout.tsx`

Add to the `<head>` section:

```tsx
export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <link rel="dns-prefetch" href="//api.psychichomily.com" />
        <link rel="preconnect" href="//api.psychichomily.com" crossOrigin="anonymous" />
        {/* If using external embeds */}
        <link rel="dns-prefetch" href="//bandcamp.com" />
        <link rel="dns-prefetch" href="//open.spotify.com" />
      </head>
      <body>...</body>
    </html>
  )
}
```

---

### 5. Defer Third-Party Scripts

**Impact**: Faster Time to Interactive (TTI)
**Effort**: 5 minutes

If analytics or other third-party scripts are added, load them after hydration:

```tsx
'use client'

import { useEffect } from 'react'

export function Analytics() {
  useEffect(() => {
    // Load analytics only after page is interactive
    if (process.env.NODE_ENV === 'production') {
      import('@/lib/analytics').then(m => m.init())
    }
  }, [])

  return null
}
```

---

### 6. Critical Image Priority

**Impact**: Faster Largest Contentful Paint (LCP)
**Effort**: 2 minutes

For above-the-fold images (logo, hero images):

```tsx
import Image from 'next/image'

// Logo in nav - always visible first
<Image
  src="/logo.png"
  alt="Psychic Homily"
  priority
  fetchPriority="high"
/>

// Below-the-fold images - lazy load (default)
<Image
  src={artistPhoto}
  alt={artistName}
  loading="lazy"
/>
```

---

### 7. Static Generation for Content Pages

**Impact**: Near-instant page loads, reduced server load
**Effort**: 10 minutes per page

For pages that don't need real-time data:

**File**: `app/blog/page.tsx`
```tsx
// Revalidate every hour
export const revalidate = 3600

// Or for truly static content
export const dynamic = 'force-static'
```

**Candidates**:
- `/blog` - Blog posts don't change frequently
- `/dj-sets` - DJ set embeds are static
- `/venues` - Venue list changes infrequently

---

### 8. Parallel Data Fetching

**Impact**: Eliminates request waterfalls
**Effort**: Varies

When fetching multiple independent resources, use `Promise.all`:

```tsx
// Before (sequential - slow)
const shows = await fetchShows()
const venues = await fetchVenues()

// After (parallel - fast)
const [shows, venues] = await Promise.all([
  fetchShows(),
  fetchVenues(),
])
```

Check these files for sequential await patterns:
- `app/api/admin/artists/[id]/discover-music/route.ts`
- `app/api/oembed/route.ts`

---

### 9. Bundle Size Monitoring

**Impact**: Catch regressions early
**Effort**: 5 minutes setup

Add to `package.json`:
```json
{
  "scripts": {
    "analyze": "ANALYZE=true next build"
  }
}
```

Install analyzer:
```bash
bun add -D @next/bundle-analyzer
```

Update `next.config.ts`:
```ts
import withBundleAnalyzer from '@next/bundle-analyzer'

const nextConfig = {
  // ... existing config
}

export default process.env.ANALYZE === 'true'
  ? withBundleAnalyzer({ enabled: true })(nextConfig)
  : nextConfig
```

---

## Priority Matrix

| Optimization | Effort | Impact | Priority |
|-------------|--------|--------|----------|
| Metadata/viewport | Low | Medium | P1 |
| Route loading states | Low | High | P1 |
| DNS prefetch | Low | Medium | P1 |
| Prefetch routes | Low | Medium | P2 |
| Image priority | Low | Medium | P2 |
| Static generation | Medium | High | P2 |
| Parallel fetching | Medium | High | P2 |
| Bundle analyzer | Low | Low | P3 |
| Defer third-party | Low | Low | P3 |

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
