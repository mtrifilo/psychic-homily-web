import { Suspense } from 'react'
import { ReleaseList } from '@/features/releases/components'
import { LoadingSpinner } from '@/components/shared'
import { listRootCanonical } from '@/lib/seo/siteMetadata'

/**
 * The canonical is FIXED at `/releases` for every query variant of this page,
 * and that is deliberate rather than incidental (PSY-1767 reviewed it and kept
 * it). `?page=`, `?type=`, `?sort=`, `?tags=`, `?year=`, `?label_id=` and
 * `?search=` all slice one list; none of them mints a second document. See the
 * site-wide pagination indexing policy on `listRootCanonical`.
 *
 * This is a static `metadata` object, so the rule is not merely followed here,
 * it cannot be broken here: there is no request to read a page number from.
 */
export const metadata = {
  title: 'Releases',
  description: 'Browse music releases - albums, EPs, singles, and more.',
  alternates: {
    canonical: listRootCanonical('/releases'),
  },
  openGraph: {
    title: 'Releases | Psychic Homily',
    description: 'Browse music releases - albums, EPs, singles, and more.',
    url: '/releases',
    type: 'website',
  },
}

export default function ReleasesPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Releases</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <ReleaseList />
        </Suspense>
      </main>
    </div>
  )
}
