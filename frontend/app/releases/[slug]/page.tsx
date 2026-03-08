import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { ReleaseDetail } from '@/components/releases'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface ReleasePageProps {
  params: Promise<{ slug: string }>
}

interface ReleaseData {
  title: string
  slug?: string
  release_type?: string
  release_year?: number | null
}

async function getRelease(slug: string): Promise<ReleaseData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/releases/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Release page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'release-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'release-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({
  params,
}: ReleasePageProps): Promise<Metadata> {
  const { slug } = await params
  const release = await getRelease(slug)

  if (release) {
    const yearSuffix = release.release_year ? ` (${release.release_year})` : ''
    return {
      title: `${release.title}${yearSuffix}`,
      description: `${release.title}${yearSuffix} - release details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/releases/${slug}`,
      },
      openGraph: {
        title: `${release.title}${yearSuffix}`,
        description: `View details for ${release.title}`,
        type: 'website',
        url: `/releases/${slug}`,
      },
    }
  }

  return {
    title: 'Release',
    description: 'View release details',
  }
}

function ReleaseLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ReleasePage({ params }: ReleasePageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Release</h1>
          <p className="text-muted-foreground">
            The release could not be found.
          </p>
        </div>
      </div>
    )
  }

  const releaseData = await getRelease(slug)

  if (!releaseData) {
    notFound()
  }

  return (
    <Suspense fallback={<ReleaseLoadingFallback />}>
      <ReleaseDetail idOrSlug={slug} />
    </Suspense>
  )
}
