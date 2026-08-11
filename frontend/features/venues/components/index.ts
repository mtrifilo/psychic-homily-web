export { VenueCard } from './VenueCard'
export { VenueSearch } from './VenueSearch'
// VenueDetail stays barrel-exported, unlike its shows/tags/scenes/releases
// peers (PSY-1772): this barrel is NOT reachable from `app/layout.tsx`, so it
// was measured to be outside the global shared chunk. It is not free, though —
// `features/festivals/admin/FestivalManagement.tsx` imports `useVenueSearch`
// from `@/features/venues`, which drags VenueDetail (and VenueBillNetwork's
// canvas) onto /admin/festivals. See the PSY-1772 PR for the follow-up.
export { VenueDetail } from './VenueDetail'
export { VenueList } from './VenueList'
export { VenueLocationCard } from './VenueLocationCard'
export { VenueShowsList } from './VenueShowsList'
export {
  VenuePastShows,
  VENUE_PAST_SHOWS_ANCHOR,
} from './VenuePastShows'
// VenueEditForm / VenueInput are intentionally NOT barrel-exported: no
// consumer needs the barrel edge (VenueCard imports ./VenueEditForm relatively;
// ShowForm deep-imports VenueInput to avoid a shows<->venues value-import
// cycle — see ShowForm.tsx). Keeping forms out of barrels also avoids inviting
// future shared-chunk hoist regressions (PSY-944/PSY-950).
export { VenueBillNetwork } from './VenueBillNetwork'
export { DeleteVenueDialog } from './DeleteVenueDialog'
export { VenueDeniedDialog } from './VenueDeniedDialog'
