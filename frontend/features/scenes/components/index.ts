export { SceneList } from './SceneList'
export { ScenePulse } from './ScenePulse'
// SceneDetailView and SceneGraph are intentionally NOT re-exported here, and
// SceneDetailView is absent from `features/scenes/index.ts` too (PSY-1772).
// `app/scenes/[slug]/page.tsx` imports SceneDetail via `dynamic()`, and
// SceneDetail.tsx deep-imports './SceneGraph'.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack groups client modules
// by how widely they are reachable, and it does not tree-shake `'use client'`
// barrels per-export. Anything reachable from `app/layout.tsx` lands in the one
// global chunk that every route loads eagerly. This barrel IS root-reachable:
// layout -> providers -> AuthContext -> features/auth -> notification-settings
// -> features/scenes -> here. So simply LISTING a component makes it global; no
// one has to import the name. That is exactly what happened to SceneGraph —
// nothing imported it by name, yet the export alone kept it, and through
// SceneGraphVisualization's static import ForceGraphView too, in that chunk.
// Removing this one line moved both out (measured; see the PSY-1772 PR for the
// before/after fingerprint). Same recipe and reason as ArtistDetail (PSY-950,
// spike PSY-944). See features/shows/components/index.ts for why
// `dynamic(ssr: true)` at the route page is the other load-bearing half.
// Guarded by features/sharedChunkBarrelGuard.test.ts.
//
// SceneGraphVisualization deliberately keeps a STATIC ForceGraphView import
// rather than `createLazyForceGraphView`, matching the peer Venue/Collection/
// Station adapters — see the note in components/graph/lazyForceGraphView.tsx.
// De-barreling achieves the eviction without reversing that decision.
export { AtlasGlobe } from './AtlasGlobe'
export { ScenePreviewPanel } from './ScenePreviewPanel'
