import { Suspense } from 'react'
import { VenueList } from '@/features/venues'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/seo/fetchSeoList'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'

export const metadata = {
  title: 'Venues',
  description: 'Browse music venues and discover upcoming shows.',
  alternates: {
    canonical: 'https://psychichomily.com/venues',
  },
  openGraph: {
    title: 'Venues | Psychic Homily',
    description: 'Browse music venues and discover upcoming shows.',
    url: '/venues',
    type: 'website',
  },
}

interface VenueListItem {
  slug: string
  name: string
}

/**
 * The endpoint's ceiling, not a product choice. `GET /venues` declares
 * `maximum:"100"` on `limit`, and huma enforces that as a 422 before the
 * handler runs. This call asked for 200, so it 422'd on every render and the
 * fail-open below dropped the `ItemList` — verified absent from the production
 * `/venues` HTML on 2026-07-29, and reproduced directly against the API. Raising
 * this past 100 needs the backend maximum raised first.
 *
 * This ALREADY truncates: production reports `total: 198` (2026-07-29), so the
 * `ItemList` covers 100 of 198, and the 98 omitted are the least active — the
 * list is sorted by upcoming show count. Nothing reports the shortfall; the
 * response carries `total` and this call discards it. That is a quieter version
 * of the same failure class, and it is why the fix is to raise the backend
 * maximum or paginate with `offset`, not to leave the cap here. Going from
 * "none" to "the 100 most active" is still strictly better than the 422 this
 * replaces, which is the only reason it ships in this state.
 */
export const VENUE_LIST_LIMIT = 100

export function getVenues(): Promise<VenueListItem[]> {
  return fetchSeoList<VenueListItem>({
    url: `${API_BASE_URL}/venues?limit=${VENUE_LIST_LIMIT}`,
    collection: 'venues',
    service: 'venues-listing',
  })
}

function VenueListLoading() {
  return (
    <div className="flex justify-center items-center py-12">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
    </div>
  )
}

export default async function VenuesPage() {
  const venues = await getVenues()

  const venuesWithSlugs = venues.filter(
    (v): v is VenueListItem & { slug: string } => !!v.slug
  )

  return (
    <>
      {venuesWithSlugs.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'Venues',
          description: 'Music venues in Phoenix and beyond.',
          listItems: venuesWithSlugs.map(venue => ({
            url: `https://psychichomily.com/venues/${venue.slug}`,
            name: venue.name,
          })),
        })} />
      )}
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Venues', url: 'https://psychichomily.com/venues' },
      ])} />
      <div className="flex min-h-screen items-start justify-center">
        <main className="w-full max-w-6xl px-4 py-8 md:px-8">
          <h1 className="text-3xl font-bold text-center mb-8">Venues</h1>
          <Suspense fallback={<VenueListLoading />}>
            <VenueList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
