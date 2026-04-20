import { Suspense } from 'react'
import type { Metadata } from 'next'
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

  return {
    title: 'Tag',
    description: 'Browse entities by tag on Psychic Homily',
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
  return (
    <Suspense fallback={<TagLoadingFallback />}>
      <TagDetail slug={slug} />
    </Suspense>
  )
}
