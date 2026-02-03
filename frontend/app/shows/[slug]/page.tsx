import { Suspense } from 'react'
import { Metadata } from 'next'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { ShowDetail } from '@/components/shows'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicEventSchema } from '@/lib/seo/jsonld'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'

interface ShowPageProps {
  params: Promise<{ slug: string }>
}

interface ShowData {
  title?: string
  date: string
  slug?: string
  ticket_url?: string
  price?: number
  venue?: {
    name: string
    address?: string
    city?: string
    state?: string
    zip_code?: string
  }
  artists?: Array<{ name: string; is_headliner?: boolean }>
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
    const venueName = show.venue?.name || 'TBA'
    const showDate = formatShowDate(show.date)
    const title = `${headliner} at ${venueName}`
    const description = `${headliner} live at ${venueName} on ${showDate}`

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

  return (
    <>
      {showData && (
        <JsonLd data={generateMusicEventSchema({
          name: showData.title,
          date: showData.date,
          venue: showData.venue,
          artists: showData.artists,
          ticket_url: showData.ticket_url,
          price: showData.price?.toString(),
          slug: showData.slug,
        })} />
      )}
      <Suspense fallback={<ShowLoadingFallback />}>
        <ShowDetail showId={slug} />
      </Suspense>
    </>
  )
}
