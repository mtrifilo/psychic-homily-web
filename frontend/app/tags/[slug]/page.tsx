import { Suspense } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { TagDetail } from '@/features/tags/components'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface TagPageProps {
  params: Promise<{ slug: string }>
}

interface TagSummaryForMetadata {
  name: string
  slug?: string
  category?: string
  usage_count?: number
}

/**
 * Fetch tag metadata for the document head. We deliberately use the lightweight
 * GET /tags/{slug} endpoint (not /tags/{slug}/detail) because we only need
 * name + usage_count for the title/description; the enriched detail call has
 * extra cost for content the client component will fetch anyway. (PSY-485)
 *
 * Returns null for 404s (expected for invalid slugs) — the page component
 * uses that signal to call `notFound()` and return a hard HTTP 404 instead
 * of a soft-404 (PSY-497).
 */
async function getTagForMetadata(
  slug: string
): Promise<TagSummaryForMetadata | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/tags/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Tag page metadata fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'tag-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'tag-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({
  params,
}: TagPageProps): Promise<Metadata> {
  const { slug } = await params
  const tag = await getTagForMetadata(slug)

  if (tag) {
    const usageCount = tag.usage_count ?? 0
    // Description ties the tag to the things people actually browse for —
    // shows/artists/releases — so the SERP snippet is concrete rather than
    // generic boilerplate.
    const description =
      usageCount > 0
        ? `Browse ${usageCount} ${usageCount === 1 ? 'entity' : 'entities'} (artists, shows, releases, and more) tagged ${tag.name} on Psychic Homily.`
        : `Discover artists, shows, and releases tagged ${tag.name} on Psychic Homily.`

    return {
      title: `${tag.name} | Tags`,
      description,
      alternates: {
        canonical: `https://psychichomily.com/tags/${slug}`,
      },
      openGraph: {
        title: `${tag.name} | Tags | Psychic Homily`,
        description,
        type: 'website',
        url: `/tags/${slug}`,
      },
    }
  }

  // Missing tag: the page component will call `notFound()` and Next.js will
  // render `app/tags/[slug]/not-found.tsx`, which owns its own metadata.
  // Returning a generic "Tag" title here would show briefly before the
  // not-found page mounts, so use the explicit not-found title instead.
  return {
    title: 'Tag not found',
    description: 'The tag you are looking for does not exist.',
  }
}

function TagLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function TagDetailPage({ params }: TagPageProps) {
  const { slug } = await params

  // Server-side existence check: if the backend doesn't know this slug, call
  // `notFound()` so Next.js returns HTTP 404 and renders the route's
  // `not-found.tsx`. Without this, the client component would render a
  // friendly "Tag Not Found" page but the response status stays 200 — a
  // soft-404 that poisons SEO, monitoring, and crawlers (PSY-497).
  //
  // We rely on `getTagForMetadata` (already called during metadata
  // generation) — but Next.js doesn't memoize across `generateMetadata` and
  // the page component, so this second fetch is unavoidable. It hits
  // `revalidate: 3600` cache in practice.
  const tag = await getTagForMetadata(slug)
  if (!tag) {
    notFound()
  }

  return (
    <Suspense fallback={<TagLoadingFallback />}>
      <TagDetail slug={slug} />
    </Suspense>
  )
}
