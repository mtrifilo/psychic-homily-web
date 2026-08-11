export { ReleaseCard } from './ReleaseCard'
// ReleaseDetail is intentionally NOT re-exported here (PSY-1772). The route page
// `app/releases/[slug]/page.tsx` imports it directly via `dynamic()` from the
// component file so Turbopack can evict it from the global shared client chunk.
// Re-adding a barrel export makes it multi-route-reachable again and re-hoists
// ReleaseDetail.tsx into the chunk that loads on every route.
export { ReleaseList } from './ReleaseList'
