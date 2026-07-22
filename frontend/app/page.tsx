import Link from 'next/link'
import { HomeShowList } from '@/features/shows'
import {
  CommunityPulseBand,
  HomeHero,
  HomeSceneGraph,
  LatestRadioShows,
} from '@/features/home'
import { Button } from '@/components/ui/button'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateWebSiteSchema } from '@/lib/seo/jsonld'

// PSY-389: logged-out discovery landing ("This is not a mirage"). Drops the old
// "Arizona Music Community" framing. NOTE: Next.js does NOT apply a layout's
// `title.template` to a page in the SAME route segment as that layout — and the
// root `page.tsx` shares the root `layout.tsx` segment — so the global
// "%s | Psychic Homily" template can't decorate this page. We therefore set the
// full title via `title.absolute` to match that template's output exactly. The
// logged-in customizable dashboard is a separate project (out of scope).
export const metadata = {
  title: { absolute: 'Discover live music | Psychic Homily' },
  description:
    'Find upcoming live shows in any city, dig into artists, labels, releases, and freeform radio — the underground, mapped link by link.',
  alternates: {
    canonical: 'https://psychichomily.com',
  },
  openGraph: {
    title: 'Psychic Homily',
    description:
      'Find upcoming live shows in any city, dig into artists, labels, releases, and freeform radio — the underground, mapped link by link.',
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
          <div className="flex w-full flex-col gap-6">
            <HomeHero />

            {/* PSY-1431: global community-pulse hairline band (Figma 1083:7).
                Same numbers for every visitor — Logged-in Dashboard owns
                personalized widgets. */}
            <CommunityPulseBand />
          </div>

          {/* Upcoming shows — the unique advantage, privileged at top. The city
              filter, geo-default, and popular-cities row all live inside the
              reused HomeShowList. */}
          <section aria-labelledby="home-shows-heading" className="flex w-full flex-col gap-4">
            <div className="flex items-center justify-between">
              <h2
                id="home-shows-heading"
                className="text-2xl font-semibold tracking-tight text-foreground"
              >
                Upcoming shows
              </h2>
              <Link
                href="/shows"
                className="text-sm font-medium text-muted-foreground transition-colors hover:text-primary hover:underline underline-offset-4"
              >
                View all shows →
              </Link>
            </div>

            <HomeShowList />

            <div className="flex justify-center pt-1.5">
              <Button asChild size="lg">
                <Link href="/shows">View more shows →</Link>
              </Button>
            </div>
          </section>

          {/* PSY-1344: "Observatory Lite" — a bounded scene-graph glimpse of
              the knowledge graph (Figma Option D, locked 2026-07-03). Lazy-
              mounts on scroll; self-hides if scene data is unavailable. */}
          <HomeSceneGraph />

          <LatestRadioShows />
        </div>
      </div>
    </>
  )
}
