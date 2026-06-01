export { ArtistCard } from './ArtistCard'
export { ArtistSearch } from './ArtistSearch'
// ArtistDetail is intentionally NOT re-exported here (PSY-950). The route page
// `app/artists/[slug]/page.tsx` imports it directly via `dynamic()` from the
// component file so Turbopack can evict it from the global shared client chunk.
// Re-adding a barrel export makes it multi-route-reachable again and silently
// re-hoists ArtistDetail.tsx (~40 KB) back into the chunk that loads on /explore.
export { ArtistList } from './ArtistList'
export { ArtistListSkeleton } from './ArtistListSkeleton'
export { ArtistShowsList } from './ArtistShowsList'
export { ArtistSimilarSidebar, ArtistGraphDialog } from './RelatedArtists'
export { BillComposition } from './BillComposition'
export { ArtistGraphVisualization } from './ArtistGraph'
export { ReportArtistButton } from './ReportArtistButton'
export { ReportArtistDialog } from './ReportArtistDialog'
