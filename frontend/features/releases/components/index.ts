export { ReleaseCard } from './ReleaseCard'
// ReleaseDetail is intentionally NOT re-exported here (PSY-1772). Its only
// consumer, `app/releases/[slug]/page.tsx`, imports the file directly via
// `dynamic()`.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack hoists any client
// module reachable from two or more route entries into one global chunk that
// every route loads eagerly. It does not tree-shake `'use client'` barrels
// per-export, so simply LISTING a component here makes it reachable from every
// route that imports this barrel for anything else — no one has to import the
// name. Re-adding the export silently puts ReleaseDetail back in that chunk.
// Same recipe and same reason as ArtistDetail (PSY-950, spike PSY-944).
export { ReleaseList } from './ReleaseList'
