import { CommunityPulseBand } from './CommunityPulseBand'
import { HomeLayoutRuntime } from './HomeLayoutRuntime'
import { HomeSceneGraph } from './HomeSceneGraph'
import { LatestRadioShows } from './LatestRadioShows'
import { NearbyShowsSection } from './NearbyShowsSection'
import { SavedShowsModule } from './SavedShowsModule'
import type { HomeLayoutDocument } from '../sections'

/** Anchor target for the zero state's "Pick from this week ↓". */
const NEARBY_SECTION_ID = 'home-shows-near-you'

/**
 * The signed-in home (PSY-2103): the viewer's saved shows in place of the
 * logged-out wordmark hero, with the general "Upcoming shows" section dropped
 * — /shows stays one click away via "Find a show" and the nearby section's
 * city link.
 *
 * A SERVER component, deliberately: it holds the map from registry id to
 * component, which is the one thing the isomorphic registry (`../sections`)
 * must not carry. Order and visibility come from `layout`, the document the
 * server read for this request, so the first paint is already this viewer's
 * page; `HomeLayoutRuntime` owns everything that changes after hydration.
 *
 * All five sections are rendered here whether or not they are visible. They
 * are client components with no server data, so an unmounted one costs an
 * element descriptor and issues no request, and showing one has to paint
 * without a round-trip. The Discover links row rides inside the nearby section
 * so the run stays contiguous when the sections move.
 */
export function SignedInHome({
  layout,
}: {
  layout: HomeLayoutDocument | null
}) {
  return (
    <HomeLayoutRuntime
      initialLayout={layout}
      sections={{
        saved_shows: <SavedShowsModule nearbySectionId={NEARBY_SECTION_ID} />,
        nearby_shows: <NearbyShowsSection id={NEARBY_SECTION_ID} />,
        community_stats: <CommunityPulseBand />,
        city_graph: <HomeSceneGraph />,
        radio_shows: <LatestRadioShows />,
      }}
    />
  )
}
