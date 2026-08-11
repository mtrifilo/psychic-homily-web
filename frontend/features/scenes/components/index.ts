export { SceneList } from './SceneList'
export { ScenePulse } from './ScenePulse'
// SceneDetailView is intentionally NOT re-exported here (PSY-1772). The route
// page `app/scenes/[slug]/page.tsx` imports it directly via `dynamic()` from
// './SceneDetail' so Turbopack can evict it from the global shared client chunk.
// Re-adding a barrel export makes it multi-route-reachable again and re-hoists
// SceneDetail.tsx (and its SceneGraph subtree) into the chunk loaded on every
// route.
// SceneGraph is intentionally NOT re-exported here (PSY-1772) for the same
// reason. Its only consumer, SceneDetail.tsx, deep-imports './SceneGraph'. A
// barrel export here reaches /atlas (via this barrel) and /scenes + the auth
// notification settings (via features/scenes/index.ts, which re-exports from
// this file), which is enough for Turbopack to hoist SceneGraph — and with it
// SceneGraphVisualization's static ForceGraphView import — into the global
// shared client chunk.
export { AtlasGlobe } from './AtlasGlobe'
export { ScenePreviewPanel } from './ScenePreviewPanel'
