import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { CollectionDetail } from '@/features/collections/components'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface CollectionPageProps {
  params: Promise<{ slug: string }>
}

interface CollectionData {
  title: string
  slug?: string
  description?: string
  creator_name?: string
}

async function getCollection(slug: string): Promise<CollectionData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/collections/${slug}`, {
      next: { revalidate: 3600 },
    })
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
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Collection</h1>
          <p className="text-muted-foreground">
            The collection could not be found.
          </p>
        </div>
      </div>
    )
  }

  const collectionData = await getCollection(slug)

  if (!collectionData) {
    notFound()
  }

  return (
    <Suspense fallback={<CollectionLoadingFallback />}>
      <CollectionDetail slug={slug} />
    </Suspense>
  )
}
