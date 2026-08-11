export { EntityTagList, AddTagDialog } from './EntityTagList'
export type { AddTagDialogProps } from './EntityTagList'
export { TagBrowse } from './TagBrowse'
// TagDetail is intentionally NOT re-exported here, and not from
// `features/tags/index.ts` either (PSY-1772). Its only consumer,
// `app/tags/[slug]/page.tsx`, imports the file directly via `dynamic()`.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack does not tree-shake
// `'use client'` barrels per-export, and anything reachable from `app/layout.tsx`
// lands in the one client chunk every route loads eagerly. This barrel IS
// root-reachable (layout -> CommandPalette -> components/shared -> here), so
// simply LISTING a component makes it global — no one has to import the name.
// Re-adding the export silently puts TagDetail back in that chunk.
// Same recipe and same reason as ArtistDetail (PSY-950, spike PSY-944). See
// features/shows/components/index.ts for why `dynamic(ssr: true)` at the route
// page is the other load-bearing half. Guarded by
// features/sharedChunkBarrelGuard.test.ts.
export { TagOfficialIndicator } from './TagOfficialIndicator'
export {
  TagFacetPanel,
  parseTagsParam,
  buildTagsParam,
  type TagFacetPanelProps,
  type TagFacetLayout,
} from './TagFacetPanel'
export { TagFacetSheet, type TagFacetSheetProps } from './TagFacetSheet'
