import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { ShowDetail } from '@/components/shows'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicEventSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface ShowPageProps {
  params: Promise<{ slug: string }>
}

interface ShowData {
  title?: string
  event_date: string
  slug?: string
  description?: string | null
  price?: number
  is_sold_out: boolean
  is_cancelled: boolean
  venues: Array<{
    name: string
    slug: string
    address?: string | null
    city: string
    state: string
  }>
  artists: Array<{
    name: string
    slug: string
    is_headliner?: boolean | null
    socials: {
      instagram?: string | null
      facebook?: string | null
      twitter?: string | null
      youtube?: string | null
      spotify?: string | null
      soundcloud?: string | null
      bandcamp?: string | null
      website?: string | null
    }
  }>
}

async function getShow(slug: string): Promise<ShowData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    // Don't report 404s - they're expected for invalid slugs
    if (res.status >= 500) {
      Sentry.captureMessage(`Show page: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'show-page' },
      extra: { slug },
    })
  }
  return null
}

function formatShowDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export async function generateMetadata({ params }: ShowPageProps): Promise<Metadata> {
  const { slug } = await params
  const show = await getShow(slug)

  if (show) {
    const headliner = show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
    const venueName = show.venues?.[0]?.name || 'TBA'
    const showDate = formatShowDate(show.event_date)
    const title = `${headliner} at ${venueName}`
    const generatedDesc = `${headliner} live at ${venueName} on ${showDate}`
    const description = show.description
      ? show.description.slice(0, 155) + (show.description.length > 155 ? '...' : '')
      : generatedDesc

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/shows/${slug}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/shows/${slug}`,
      },
    }
  }

  return {
    title: 'Show',
    description: 'View show details',
  }
}

function ShowLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ShowPage({ params }: ShowPageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Show</h1>
          <p className="text-muted-foreground">
            The show could not be found.
          </p>
        </div>
      </div>
    )
  }

  const showData = await getShow(slug)

  if (!showData) {
    notFound()
  }

  const headliner = showData.artists?.find(a => a.is_headliner)?.name || showData.artists?.[0]?.name || 'Live Music'
  const showName = showData.title || `${headliner} at ${showData.venues?.[0]?.name || 'TBA'}`

  return (
    <>
      <JsonLd data={generateMusicEventSchema({
        name: showData.title,
        date: showData.event_date,
        description: showData.description ?? undefined,
        is_cancelled: showData.is_cancelled,
        is_sold_out: showData.is_sold_out,
        venue: showData.venues?.[0] ? {
          name: showData.venues[0].name,
          slug: showData.venues[0].slug,
          address: showData.venues[0].address ?? undefined,
          city: showData.venues[0].city,
          state: showData.venues[0].state,
        } : undefined,
        artists: showData.artists?.map(a => ({
          name: a.name,
          slug: a.slug,
          is_headliner: a.is_headliner ?? undefined,
          socials: a.socials,
        })),
        price: showData.price,
        slug: showData.slug,
      })} />
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Shows', url: 'https://psychichomily.com/shows' },
        { name: showName, url: `https://psychichomily.com/shows/${slug}` },
      ])} />
      <Suspense fallback={<ShowLoadingFallback />}>
        <ShowDetail showId={slug} />
      </Suspense>
    </>
  )
}
