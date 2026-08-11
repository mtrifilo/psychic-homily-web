export { SceneList } from './SceneList'
export { ScenePulse } from './ScenePulse'
// SceneDetailView and SceneGraph are intentionally NOT re-exported here, and
// SceneDetailView is absent from `features/scenes/index.ts` too (PSY-1772).
// `app/scenes/[slug]/page.tsx` imports SceneDetail via `dynamic()`, and
// SceneDetail.tsx deep-imports './SceneGraph'.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack hoists any client
// module reachable from two or more route entries into one global chunk that
// every route loads eagerly. It does not tree-shake `'use client'` barrels
// per-export, so simply LISTING a component here makes it reachable from every
// route that imports this barrel for anything else — no one has to import the
// name. That is exactly what happened to SceneGraph: nothing imported it by
// name, but the export alone kept it (and, through SceneGraphVisualization's
// static import, ForceGraphView) in the chunk loaded on all 96 route shells.
// Removing this one line moved ForceGraphView to 4. Same recipe and same
// reason as ArtistDetail (PSY-950, spike PSY-944).
//
// SceneGraphVisualization deliberately keeps a STATIC ForceGraphView import
// rather than `createLazyForceGraphView`, matching the peer Venue/Collection/
// Station adapters — see the note in components/graph/lazyForceGraphView.tsx.
// De-barreling achieves the eviction without reversing that decision.
export { AtlasGlobe } from './AtlasGlobe'
export { ScenePreviewPanel } from './ScenePreviewPanel'
