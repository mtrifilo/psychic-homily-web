import { cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import StationDetail from '../../_components/StationDetail'
import { API_BASE_URL } from '@/lib/api-base'
import { reportServerFetchFailure } from '@/lib/ssr/reportServerFetchFailure'
import { serverReadHeaders } from '@/lib/ssr/serverReadHeaders'

interface SubStationPageProps {
  params: Promise<{ 'station-slug': string; 'sub-slug': string }>
}

interface StationData {
  name: string
  slug?: string
  description?: string | null
  city?: string | null
  state?: string | null
}

// PSY-674: route for stations nested under a network. URL shape is
// /radio/{network-slug}/channel/{local-slug} where local-slug is the
// station's slug with the "{network-slug}-" prefix stripped (e.g.
// /radio/wfmu/channel/drummer resolves to the wfmu-drummer station). The
// literal "channel" segment disambiguates from show pages, which live at
// /radio/{station-slug}/{show-slug}. Reconstruction mirrors the stripping
// done by getStationDetailUrl.
function reconstructStationSlug(networkSlug: string, subSlug: string): string {
  const expectedPrefix = `${networkSlug}-`
  return subSlug.startsWith(expectedPrefix) ? subSlug : `${networkSlug}-${subSlug}`
}

// Wrapped with `React.cache()` so `generateMetadata` and the page body
// share ONE backend fetch per request.
const getStation = cache(async (slug: string): Promise<StationData | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/radio-stations/${slug}`, {
      headers: serverReadHeaders(),
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    reportServerFetchFailure({
      response: res,
      service: 'radio-substation-page',
      pathFamily: '/radio-stations/{slug}',
      notFoundIsAnswer: true,
      context: { slug },
    })
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'radio-substation-page' },
      extra: { slug },
    })
  }
  return null
})

export async function generateMetadata({ params }: SubStationPageProps): Promise<Metadata> {
  const { 'station-slug': networkSlug, 'sub-slug': subSlug } = await params
  const fullSlug = reconstructStationSlug(networkSlug, subSlug)
  const station = await getStation(fullSlug)
  const canonical = `https://psychichomily.com/radio/${networkSlug}/channel/${subSlug}`

  if (station) {
    const location = [station.city, station.state].filter(Boolean).join(', ')
    const description = station.description
      ? station.description.slice(0, 155) + (station.description.length > 155 ? '...' : '')
      : `${station.name} radio station${location ? ` from ${location}` : ''} on Psychic Homily`
    const title = `${station.name} — Radio`

    return {
      title,
      description,
      alternates: { canonical },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/radio/${networkSlug}/channel/${subSlug}`,
      },
    }
  }

  return {
    title: 'Radio Station',
    description: 'View radio station details, shows, and playlists',
  }
}

export default async function SubStationPage({ params }: SubStationPageProps) {
  const { 'station-slug': networkSlug, 'sub-slug': subSlug } = await params
  const fullSlug = reconstructStationSlug(networkSlug, subSlug)
  if (!(await getStation(fullSlug))) {
    notFound()
  }
  return <StationDetail stationSlug={fullSlug} />
}
