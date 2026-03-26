import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { CrateDetail } from '@/features/crates/components'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface CratePageProps {
  params: Promise<{ slug: string }>
}

interface CrateData {
  title: string
  slug?: string
  description?: string
  creator_name?: string
}

async function getCrate(slug: string): Promise<CrateData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/crates/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Crate page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'crate-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'crate-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({
  params,
}: CratePageProps): Promise<Metadata> {
  const { slug } = await params
  const crate = await getCrate(slug)

  if (crate) {
    const description = crate.description
      ? crate.description.slice(0, 160)
      : `${crate.title} - a curated crate on Psychic Homily`

    return {
      title: crate.title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/crates/${slug}`,
      },
      openGraph: {
        title: crate.title,
        description,
        type: 'website',
        url: `/crates/${slug}`,
      },
    }
  }

  return {
    title: 'Crate',
    description: 'View crate details',
  }
}

function CrateLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function CratePage({ params }: CratePageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Crate</h1>
          <p className="text-muted-foreground">
            The crate could not be found.
          </p>
        </div>
      </div>
    )
  }

  const crateData = await getCrate(slug)

  if (!crateData) {
    notFound()
  }

  return (
    <Suspense fallback={<CrateLoadingFallback />}>
      <CrateDetail slug={slug} />
    </Suspense>
  )
}
