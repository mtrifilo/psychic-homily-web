import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { cookies } from 'next/headers'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary, dehydrate } from '@tanstack/react-query'
// Imported from the component FILE, not the feature barrel: a barrel re-export
// keeps CollectionDetail multi-route-reachable under Turbopack and re-hoists its
// lazily-split libs (@dnd-kit / marked / dompurify) into the global shared
// chunk. See PSY-951 / PSY-944 and the note in components/index.ts.
import { CollectionDetail } from '@/features/collections/components/CollectionDetail'
import type { CollectionDetail as CollectionDetailData } from '@/features/collections/types'
import { getQueryClient, queryKeys } from '@/lib/queryClient'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface CollectionPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Forwards the viewer's auth cookie so SSR sees the same view as the
 * browser — without this, owners of private collections would 404 on their
 * own pages because the page route bypasses the /api proxy that normally
 * attaches the cookie. Mirrors the cookie-forward pattern in
 * app/api/[...path]/route.ts.
 *
 * Wrapped in `React.cache()` so `generateMetadata` and the page body share
 * one round-trip per request; the same payload then hydrates the TanStack
 * Query cache under `queryKeys.collections.detail(slug)` so the client
 * `useCollection` hook resolves from cache instead of refetching.
 *
 * Privacy: backend `GetBySlug` returns 403 for unauthorized access to
 * private collections; we treat any non-2xx as null and fall through to
 * `notFound()`, so the SSR payload never contains data the viewer isn't
 * authorized to see. Authenticated requests use `cache: 'no-store'` to
 * avoid cross-user cache pollution; anonymous requests stay on ISR.
 */
const getCollection = cache(
  async (slug: string): Promise<CollectionDetailData | null> => {
    const cookieStore = await cookies()
    const authToken = cookieStore.get('auth_token')

    const fetchInit: RequestInit = authToken
      ? {
          headers: { Cookie: `auth_token=${authToken.value}` },
          cache: 'no-store',
        }
      : { next: { revalidate: 3600 } }

    try {
      const res = await fetch(`${API_BASE_URL}/collections/${slug}`, fetchInit)
      if (res.ok) {
        return res.json()
      }
      if (res.status >= 500) {
        Sentry.captureMessage(`Collection page fetch error: ${res.status}`, {
          level: 'error',
          tags: { service: 'collection-page' },
          extra: { slug, status: res.status },
        })
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'collection-page' },
        extra: { slug },
      })
    }
    return null
  }
)

export async function generateMetadata({
  params,
}: CollectionPageProps): Promise<Metadata> {
  const { slug } = await params
  const collection = await getCollection(slug)

  if (collection) {
    const description = collection.description
      ? collection.description.slice(0, 160)
      : `${collection.title} - a curated collection on Psychic Homily`

    return {
      title: collection.title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/collections/${slug}`,
      },
      openGraph: {
        title: collection.title,
        description,
        type: 'website',
        url: `/collections/${slug}`,
      },
    }
  }

  return {
    title: 'Collection',
    description: 'View collection details',
  }
}

function CollectionLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function CollectionPage({ params }: CollectionPageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  const collectionData = await getCollection(slug)

  if (!collectionData) {
    notFound()
  }

  // `cache()` above guarantees the network call already happened, so the
  // sync `queryFn` is a no-op cache write that just seeds the dehydrated
  // entry the client `useCollection` hook will pick up.
  const queryClient = getQueryClient()
  await queryClient.prefetchQuery({
    queryKey: queryKeys.collections.detail(slug),
    queryFn: () => collectionData,
  })

  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <Suspense fallback={<CollectionLoadingFallback />}>
        <CollectionDetail slug={slug} />
      </Suspense>
    </HydrationBoundary>
  )
}
