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
 * The signed-in home: the viewer's saved shows in place of the
 * logged-out wordmark hero, with the general "Upcoming shows" section dropped
 * — /shows stays one click away via "Find a show" and the nearby section's
 * city link.
 *
 * It holds the map from registry id to component, which is the one thing the
 * isomorphic registry (`../sections`) must not carry. Order and visibility
 * come from `layout`, the document the server read for this request, so the
 * first paint is already this viewer's page; `HomeLayoutRuntime` owns
 * everything that changes after hydration.
 *
 * ISOMORPHIC, not server-only. It renders as a server component under
 * `HomeContentSlot` and as a CLIENT component under `HomeVariantSwitch`, the
 * fallback used when the server's viewer read failed. Nothing server-only may
 * be called here: that path fires on a backend blip, so a `cookies()` or a
 * `fetch` added in this file would break only in production. An OMITTED
 * `layout` means the server had no answer, which is not the same as `null`
 * ("this viewer has no stored layout") and must not be passed as one.
 *
 * All five sections are handed over whether or not they are visible, because
 * showing one has to paint without a round-trip. A section the runtime does
 * not render mounts nothing and issues no request; its client reference is
 * still serialized into the RSC payload, so hiding a section trims the page,
 * not the payload. The Discover links row rides inside the nearby section so
 * the run stays contiguous when the sections move.
 */
export function SignedInHome({
  layout,
}: {
  layout?: HomeLayoutDocument | null
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
