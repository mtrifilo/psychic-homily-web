import { Suspense } from 'react'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateWebSiteSchema } from '@/lib/seo/jsonld'
import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'
import { HomeContentSlot } from './HomeContentSlot'

// PSY-389: logged-out discovery landing ("This is not a mirage"). Drops the old
// "Arizona Music Community" framing. NOTE: Next.js does NOT apply a layout's
// `title.template` to a page in the SAME route segment as that layout — and the
// root `page.tsx` shares the root `layout.tsx` segment — so the global
// "%s | Psychic Homily" template can't decorate this page. We therefore set the
// full title via `title.absolute` to match that template's output exactly.
//
// PSY-2103: the page now has two viewer variants, picked server-side inside
// `<HomeContentSlot>`. The metadata above is the anonymous page's and is shared
// by both: it is what crawlers and link unfurls see, and they are never signed
// in. The signed-in variant's ordering/visibility registry is PSY-2104.
export const metadata = {
  title: { absolute: 'Discover live music | Psychic Homily' },
  description: SITE_DESCRIPTION,
  alternates: {
    canonical: 'https://psychichomily.com',
  },
  openGraph: {
    title: 'Psychic Homily',
    description: SITE_DESCRIPTION,
    url: '/',
    type: 'website',
  },
}

export default function Home() {
  return (
    <>
      <JsonLd data={generateWebSiteSchema()} />
      <div className="flex w-full justify-center">
        <div className="flex w-full max-w-6xl flex-col gap-14 px-4 pb-16 pt-12 md:px-8">
          {/* The viewer read is confined to this boundary so the route keeps
              its prerendered shell (see HomeContentSlot). `null` while it
              streams, matching the root layout's AuthHydrator boundary: a
              placeholder that guessed a variant would be wrong for half the
              viewers and would shift when the real one arrived. */}
          <Suspense fallback={null}>
            <HomeContentSlot />
          </Suspense>
        </div>
      </div>
    </>
  )
}
