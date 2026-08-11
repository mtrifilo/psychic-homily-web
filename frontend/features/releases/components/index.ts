export { ReleaseCard } from './ReleaseCard'
// ReleaseDetail is intentionally NOT re-exported here (PSY-1772). Its only
// consumer, `app/releases/[slug]/page.tsx`, imports the file directly via
// `dynamic()`.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack does not tree-shake
// `'use client'` barrels per-export, and anything reachable from `app/layout.tsx`
// lands in the one client chunk every route loads eagerly. This barrel was
// root-reachable, so simply LISTING a component made it global — no one had to
// import the name. Re-adding the export silently puts ReleaseDetail back there.
// Same recipe and same reason as ArtistDetail (PSY-950, spike PSY-944). See
// features/shows/components/index.ts for why `dynamic(ssr: true)` at the route
// page is the other load-bearing half. Guarded by
// features/sharedChunkBarrelGuard.test.ts.
export { ReleaseList } from './ReleaseList'
