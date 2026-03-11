import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { LabelDetail } from '@/features/labels/components'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface LabelPageProps {
  params: Promise<{ slug: string }>
}

interface LabelData {
  name: string
  slug?: string
  city?: string | null
  state?: string | null
  status?: string
}

async function getLabel(slug: string): Promise<LabelData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/labels/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Label page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'label-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'label-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({
  params,
}: LabelPageProps): Promise<Metadata> {
  const { slug } = await params
  const label = await getLabel(slug)

  if (label) {
    const locationSuffix =
      label.city && label.state ? ` - ${label.city}, ${label.state}` : ''
    return {
      title: `${label.name}${locationSuffix}`,
      description: `${label.name}${locationSuffix} - label details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/labels/${slug}`,
      },
      openGraph: {
        title: `${label.name}${locationSuffix}`,
        description: `View details for ${label.name}`,
        type: 'website',
        url: `/labels/${slug}`,
      },
    }
  }

  return {
    title: 'Label',
    description: 'View label details',
  }
}

function LabelLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function LabelPage({ params }: LabelPageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Label</h1>
          <p className="text-muted-foreground">
            The label could not be found.
          </p>
        </div>
      </div>
    )
  }

  const labelData = await getLabel(slug)

  if (!labelData) {
    notFound()
  }

  return (
    <Suspense fallback={<LabelLoadingFallback />}>
      <LabelDetail idOrSlug={slug} />
    </Suspense>
  )
}
