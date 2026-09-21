import Link from 'next/link'
import { HomeShowList } from '@/features/shows'
import { Button } from '@/components/ui/button'
import { CommunityPulseBand } from './CommunityPulseBand'
import { HomeHero } from './HomeHero'
import { HomeSceneGraph } from './HomeSceneGraph'
import { LatestRadioShows } from './LatestRadioShows'

/**
 * The logged-out discovery landing (PSY-389), lifted out of `app/page.tsx`
 * unchanged when the signed-in variant landed (PSY-2103). Its output is the
 * anonymous homepage's contract: change it only to change that page.
 */
export function AnonymousHome() {
  return (
    <>
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
    </>
  )
}
