// ShowDetail is intentionally NOT re-exported here, and not from
// `features/shows/index.ts` either (PSY-1772). Its only consumer,
// `app/shows/[slug]/page.tsx`, imports the file directly via `dynamic()`.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack hoists any client
// module reachable from two or more route entries into one global chunk that
// every route loads eagerly. It does not tree-shake `'use client'` barrels
// per-export, so simply LISTING a component here makes it reachable from every
// route that imports this barrel for anything else — no one has to import the
// name. Re-adding the export silently puts ShowDetail back in that chunk.
// Same recipe and same reason as ArtistDetail (PSY-950, spike PSY-944).
export { ShowHeader } from './ShowHeader'
export { ShowStatusStripe } from './ShowStatusStripe'
export { ShowActions } from './ShowActions'
export { ShowCard } from './ShowCard'
export type { ShowCardDensity, ShowCardProps } from './ShowCard'
// ShowForm IS barrel-exported deliberately (unlike the artists/venues form
// components): its external consumers (VenueCard, VenueShowsList, app pages)
// import it via '@/features/shows', and it was already statically reachable
// from this barrel on main via ShowCard, so reachability is unchanged.
export { ShowForm } from './ShowForm'
export { ShowList } from './ShowList'
export { ShowListSkeleton } from './ShowListSkeleton'
export { HomeShowList } from './HomeShowList'
export { DeleteShowDialog } from './DeleteShowDialog'
export { PublishShowDialog } from './PublishShowDialog'
export { UnpublishShowDialog } from './UnpublishShowDialog'
export { MakePrivateDialog } from './MakePrivateDialog'
export { ExportShowButton } from './ExportShowButton'
export { ReportShowButton } from './ReportShowButton'
export { ReportShowDialog } from './ReportShowDialog'
export { ShowStatusBadge } from './ShowStatusBadge'
export {
  ShowSubmissionsConsole,
  ShowSubmissionsLoading,
} from './ShowSubmissionsConsole'
export { CompactShowRow } from './CompactShowRow'
export { AIFormFiller } from './AIFormFiller'
export { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
export type {
  ShowListContext,
  ShowListFeaturePolicy,
} from './showListFeaturePolicy'
