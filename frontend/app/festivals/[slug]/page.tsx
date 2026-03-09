import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { FestivalDetail } from '@/components/festivals'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface FestivalPageProps {
  params: Promise<{ slug: string }>
}

interface FestivalData {
  name: string
  slug?: string
  city?: string | null
  state?: string | null
  start_date?: string
  end_date?: string
  status?: string
}

async function getFestival(slug: string): Promise<FestivalData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/festivals/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Festival page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'festival-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'festival-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({
  params,
}: FestivalPageProps): Promise<Metadata> {
  const { slug } = await params
  const festival = await getFestival(slug)

  if (festival) {
    const locationSuffix =
      festival.city && festival.state ? ` - ${festival.city}, ${festival.state}` : ''
    return {
      title: `${festival.name}${locationSuffix}`,
      description: `${festival.name}${locationSuffix} - festival details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/festivals/${slug}`,
      },
      openGraph: {
        title: `${festival.name}${locationSuffix}`,
        description: `View details for ${festival.name}`,
        type: 'website',
        url: `/festivals/${slug}`,
      },
    }
  }

  return {
    title: 'Festival',
    description: 'View festival details',
  }
}

function FestivalLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function FestivalPage({ params }: FestivalPageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Festival</h1>
          <p className="text-muted-foreground">
            The festival could not be found.
          </p>
        </div>
      </div>
    )
  }

  const festivalData = await getFestival(slug)

  if (!festivalData) {
    notFound()
  }

  return (
    <Suspense fallback={<FestivalLoadingFallback />}>
      <FestivalDetail idOrSlug={slug} />
    </Suspense>
  )
}
