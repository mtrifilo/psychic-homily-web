import { CommunityPulseBand } from './CommunityPulseBand'
import { HomeLayoutRuntime } from './HomeLayoutRuntime'
import { HomeSceneGraph } from './HomeSceneGraph'
import { LatestRadioShows } from './LatestRadioShows'
import { LibraryUpdates } from './LibraryUpdates'
import { NearbyShowsSection } from './NearbyShowsSection'
import { SavedShowsModule } from './SavedShowsModule'
import type { HomeLayoutDocument } from '../sections'

/** The nearby section's element id. */
const NEARBY_SECTION_ID = 'home-shows-near-you'

/**
 * The signed-in home: the viewer's saved shows in place of the
 * logged-out wordmark hero, with the general "Upcoming shows" section dropped.
 * /shows stays one click away through the city link, which cascades from the
 * nearby section to the saved-shows footer to the page-end row so some
 * surface always carries it, and through "Find a show" in the zero-saves
 * state.
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
 * Every section is handed over whether or not it is visible, because
 * showing one has to paint without a round-trip. A section the runtime does
 * not render mounts nothing and issues no request; its client reference is
 * still serialized into the RSC payload, so hiding a section trims the page,
 * not the payload. The Discover links row is no section of its own: it rides
 * inside the nearby section and relocates with the city link when that section
 * is hidden (see `resolveHomeLinkSlots`).
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
        saved_shows: <SavedShowsModule />,
        library_updates: <LibraryUpdates />,
        nearby_shows: <NearbyShowsSection id={NEARBY_SECTION_ID} />,
        community_stats: <CommunityPulseBand />,
        city_graph: <HomeSceneGraph narrowForm="linkRow" />,
        radio_shows: <LatestRadioShows variant="rows" />,
      }}
    />
  )
}
