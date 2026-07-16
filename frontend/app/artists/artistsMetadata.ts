import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import { createBuildTimeApiSignal } from '@/lib/build-time-api'

export interface ArtistListItem {
  slug: string
  name: string
}

interface ArtistsApiResponse {
  artists: ArtistListItem[]
}

export async function getArtistsForMetadata(
  fetchArtists: typeof fetch = fetch
): Promise<ArtistListItem[]> {
  try {
    const response = await fetchArtists(`${API_BASE_URL}/artists`, {
      next: { revalidate: 3600 },
      signal: createBuildTimeApiSignal(),
    })
    if (response.ok) {
      const data: ArtistsApiResponse = await response.json()
      return data.artists ?? []
    }
    if (response.status >= 500) {
      Sentry.captureMessage(
        `Artists listing: API returned ${response.status}`,
        {
          level: 'error',
          tags: { service: 'artists-listing' },
          extra: { status: response.status },
        }
      )
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'artists-listing' },
    })
  }
  return []
}
