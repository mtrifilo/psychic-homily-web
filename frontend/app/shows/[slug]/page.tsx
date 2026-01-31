import { Suspense } from 'react'
import { Metadata } from 'next'
import { Loader2 } from 'lucide-react'
import { ShowDetail } from '@/components/ShowDetail'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'

interface ShowPageProps {
  params: Promise<{ slug: string }>
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
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      const show = await res.json()
      const headliner = show.artists?.find((a: { is_headliner?: boolean }) => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
      const venueName = show.venue?.name || 'TBA'
      const showDate = formatShowDate(show.date)
      const title = `${headliner} at ${venueName}`
      const description = `${headliner} live at ${venueName} on ${showDate}`

      return {
        title,
        description,
        openGraph: {
          title,
          description,
          type: 'website',
          url: `/shows/${slug}`,
        },
      }
    }
  } catch {
    // Fall through to default metadata
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

  return (
    <Suspense fallback={<ShowLoadingFallback />}>
      <ShowDetail showId={slug} />
    </Suspense>
  )
}
