# SEO Improvements Checklist

## Overview

This document tracks SEO improvements beyond the slug-based URL implementation completed on 2026-01-30. These enhancements will improve search engine visibility, social sharing, and rich results for concert/event listings.

---

## Current State

### Already Implemented
- **SEO-friendly slugs** - Artists, venues, and shows use human-readable URLs
- **Basic meta tags** - Title/description on all major pages with template support
- **Sitemap** - Static pages, blog posts, DJ sets, shows, venues, artists included (dynamic)
- **robots.txt** - Properly blocks private routes (admin, profile, auth)
- **Open Graph basics** - Site name, type, locale configured in root layout
- **Metadata base URL** - Set to `https://psychichomily.com`
- **JSON-LD structured data** - MusicEvent, MusicVenue, MusicGroup, BlogPosting, Organization schemas
- **Default OG image** - `/og-image.jpg` for social sharing
- **Canonical URLs** - All detail pages (shows, artists, venues, blog) have canonical URLs

### Key Files
- Root metadata: `frontend/app/layout.tsx`
- JSON-LD helpers: `frontend/lib/seo/jsonld.ts`
- JSON-LD component: `frontend/components/seo/JsonLd.tsx`
- Sitemap: `frontend/app/sitemap.ts`
- Robots: `frontend/app/robots.ts`
- Default OG image: `frontend/public/og-image.jpg`

---

## Priority 1: HIGH (Pre-Launch)

### 1.1 Integrate JSON-LD Structured Data
**Status**: Complete (2026-01-31)
**Impact**: Rich results in Google for events, improved search visibility

**Implementation**:
- Created reusable `<JsonLd>` component at `frontend/components/seo/JsonLd.tsx`
- Added `generateMusicGroupSchema()` function to `frontend/lib/seo/jsonld.ts`

**Completed Tasks**:
- [x] Add `MusicEventSchema` to show detail pages (`frontend/app/shows/[slug]/page.tsx`)
- [x] Add `MusicVenueSchema` to venue detail pages (`frontend/app/venues/[slug]/page.tsx`)
- [x] Add `MusicGroupSchema` to artist detail pages (`frontend/app/artists/[slug]/page.tsx`)
- [x] Add `OrganizationSchema` to root layout (`frontend/app/layout.tsx`)
- [x] Add `BlogPostingSchema` to blog post pages (`frontend/app/blog/[slug]/page.tsx`)
- [ ] Test with Google's Rich Results Test: https://search.google.com/test/rich-results

**Usage pattern**:
```tsx
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicEventSchema } from '@/lib/seo/jsonld'

export default async function ShowPage({ params }) {
  const show = await getShow(params.slug)

  return (
    <>
      <JsonLd data={generateMusicEventSchema({
        name: show.title,
        date: show.date,
        venue: show.venue,
        artists: show.artists,
        ticket_url: show.ticket_url,
        price: show.price?.toString(),
        slug: show.slug,
      })} />
      {/* page content */}
    </>
  )
}
```

### 1.2 Add Open Graph Images
**Status**: Complete (2026-01-31)
**Impact**: Better engagement when content is shared on social media

**Completed Tasks**:
- [x] Create default OG image for general pages
  - Saved to `frontend/public/og-image.jpg`
- [x] Add default image to root layout metadata
- [x] Add Twitter image to root layout metadata
- [ ] (Optional) Set up dynamic OG image generation for shows using `@vercel/og` or similar
- [ ] Test with Facebook Sharing Debugger: https://developers.facebook.com/tools/debug/
- [ ] Test with Twitter Card Validator: https://cards-dev.twitter.com/validator

---

## Priority 2: MEDIUM (Launch Week)

### 2.1 Extend Sitemap to Include Dynamic Content
**Status**: Complete (2026-01-31)
**Impact**: Search engines can discover all shows, artists, and venues

**Implementation**:
- Added `GET /artists` endpoint to backend (`backend/internal/api/handlers/artist.go`)
- Updated `frontend/app/sitemap.ts` to be async and fetch from API
- Sitemap now includes shows, venues, artists, blog posts, and DJ sets
- Data is fetched in parallel with 1-hour revalidation

**Completed Tasks**:
- [x] Add API endpoint or server function to fetch all published shows (existing `GET /shows`)
- [x] Add API endpoint or server function to fetch all artists (new `GET /artists`)
- [x] Add API endpoint or server function to fetch all venues (existing `GET /venues`)
- [x] Update `sitemap.ts` to include:
  - [x] All show pages with `changeFrequency: 'weekly'`, `priority: 0.8`
  - [x] All artist pages with `changeFrequency: 'monthly'`, `priority: 0.6`
  - [x] All venue pages with `changeFrequency: 'monthly'`, `priority: 0.6`
- [ ] Verify sitemap at `https://psychichomily.com/sitemap.xml`
- [ ] Submit sitemap to Google Search Console

**Note**: For large numbers of entries, consider splitting into multiple sitemaps with a sitemap index.

### 2.2 Add Canonical URLs
**Status**: Complete (2026-01-31)
**Impact**: Prevents duplicate content issues

**Completed Tasks**:
- [x] Add `alternates.canonical` to show detail page metadata
- [x] Add `alternates.canonical` to artist detail page metadata
- [x] Add `alternates.canonical` to venue detail page metadata
- [x] Add `alternates.canonical` to blog post page metadata

**Implementation pattern**:
```tsx
export async function generateMetadata({ params }): Promise<Metadata> {
  return {
    // ... other metadata
    alternates: {
      canonical: `https://psychichomily.com/shows/${params.slug}`
    }
  }
}
```

---

## Priority 3: LOW (Post-Launch)

### 3.1 Dynamic OG Images for Events
**Status**: Not started
**Impact**: Eye-catching social previews with event-specific info

**Tasks**:
- [ ] Install `@vercel/og` package
- [ ] Create OG image route at `frontend/app/api/og/route.tsx`
- [ ] Design template showing: artist name, venue, date, Psychic Homily branding
- [ ] Update show detail pages to use dynamic OG image URL
- [ ] Consider caching strategy for generated images

### 3.2 Image Alt Text Audit
**Status**: Not started
**Impact**: Accessibility and image search visibility

**Tasks**:
- [ ] Audit all `<Image>` and `<img>` tags for meaningful alt text
- [ ] Add alt text to artist photos
- [ ] Add alt text to venue images
- [ ] Add alt text to show flyers/posters

### 3.3 Performance SEO (Core Web Vitals)
**Status**: Not started
**Impact**: Google ranking factor

**Tasks**:
- [ ] Run Lighthouse audit on key pages
- [ ] Optimize Largest Contentful Paint (LCP)
  - [ ] Ensure images use `next/image` with proper sizing
  - [ ] Add `priority` prop to above-the-fold images
- [ ] Optimize Cumulative Layout Shift (CLS)
  - [ ] Add explicit dimensions to images
  - [ ] Reserve space for dynamic content
- [ ] Optimize Interaction to Next Paint (INP)
  - [ ] Audit client-side interactivity for long tasks
- [ ] Check font loading strategy (Geist fonts)

---

## Verification Checklist

After implementing changes, verify with these tools:

- [ ] **Google Rich Results Test**: https://search.google.com/test/rich-results
- [ ] **Schema Markup Validator**: https://validator.schema.org/
- [ ] **Facebook Sharing Debugger**: https://developers.facebook.com/tools/debug/
- [ ] **Twitter Card Validator**: https://cards-dev.twitter.com/validator
- [ ] **Google PageSpeed Insights**: https://pagespeed.web.dev/
- [ ] **Lighthouse** (Chrome DevTools > Lighthouse > SEO)
- [ ] **Google Search Console**: Submit sitemap, check indexing status

---

## Resources

- [Next.js Metadata API](https://nextjs.org/docs/app/building-your-application/optimizing/metadata)
- [Schema.org Event](https://schema.org/Event)
- [Schema.org MusicEvent](https://schema.org/MusicEvent)
- [Google Search Central - Events](https://developers.google.com/search/docs/appearance/structured-data/event)
- [Open Graph Protocol](https://ogp.me/)
- [Vercel OG Image Generation](https://vercel.com/docs/functions/og-image-generation)
