'use client'

import { CommunityPulseBand } from './CommunityPulseBand'
import { HomeDiscoverLinks } from './HomeDiscoverLinks'
import { HomeSceneGraph } from './HomeSceneGraph'
import { LatestRadioShows } from './LatestRadioShows'
import { NearbyShowsSection } from './NearbyShowsSection'
import { SavedShowsModule } from './SavedShowsModule'

/** Anchor target for the zero state's "Pick from this week ↓". */
const NEARBY_SECTION_ID = 'home-shows-near-you'

/**
 * The signed-in home (PSY-2103): the viewer's saved shows in place of the
 * logged-out wordmark hero, with the general "Upcoming shows" section dropped
 * — /shows stays one click away via "Find a show" and the nearby section's
 * city link.
 *
 * The five sections are flat siblings in one parent, in their default order, so
 * PSY-2104 can reorder and hide them from a registry without restructuring the
 * tree. Nothing here reads a per-viewer layout preference yet, and no section
 * header carries a gear.
 */
export function SignedInHome() {
  return (
    <>
      <SavedShowsModule nearbySectionId={NEARBY_SECTION_ID} />
      <NearbyShowsSection id={NEARBY_SECTION_ID} />
      <HomeDiscoverLinks className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm" />
      <CommunityPulseBand />
      <HomeSceneGraph />
      <LatestRadioShows />
    </>
  )
}
