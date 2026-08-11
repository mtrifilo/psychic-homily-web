// ShowDetail is intentionally NOT re-exported here, and not from
// `features/shows/index.ts` either (PSY-1772). Its only consumer,
// `app/shows/[slug]/page.tsx`, imports the file directly via `dynamic()`.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack does not tree-shake
// `'use client'` barrels per-export, and anything reachable from `app/layout.tsx`
// lands in the one client chunk every route loads eagerly. This barrel IS
// root-reachable (layout -> CommandPalette -> components/shared -> SaveButton ->
// features/shows -> here), so simply LISTING a component makes it global — no
// one has to import the name. Re-adding the export silently puts ShowDetail
// back in that chunk.
// Same recipe and same reason as ArtistDetail (PSY-950, spike PSY-944).
//
// The route page pairs this absence with `dynamic(ssr: true)`. Both halves are
// load-bearing: de-barreling is what evicts the module from the global chunk,
// and `dynamic()` is what keeps it out of the route's own eager bundle. The
// explicit `ssr: true` matches `next/dynamic`'s default on purpose — it records
// that SSR must NOT be turned off here (these are crawlable entity pages that
// server-render through prefetchEntity + HydrationBoundary).
// Guarded by features/sharedChunkBarrelGuard.test.ts.
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
