export { EntityTagList, AddTagDialog } from './EntityTagList'
export type { AddTagDialogProps } from './EntityTagList'
export { TagBrowse } from './TagBrowse'
// TagDetail is intentionally NOT re-exported here, and not from
// `features/tags/index.ts` either (PSY-1772). Its only consumer,
// `app/tags/[slug]/page.tsx`, imports the file directly via `dynamic()`.
//
// WHY, and why `dynamic()` alone is not enough: Turbopack hoists any client
// module reachable from two or more route entries into one global chunk that
// every route loads eagerly. It does not tree-shake `'use client'` barrels
// per-export, so simply LISTING a component here makes it reachable from every
// route that imports this barrel for anything else — no one has to import the
// name. Re-adding the export silently puts TagDetail back in that chunk.
// Same recipe and same reason as ArtistDetail (PSY-950, spike PSY-944).
export { TagOfficialIndicator } from './TagOfficialIndicator'
export {
  TagFacetPanel,
  parseTagsParam,
  buildTagsParam,
  type TagFacetPanelProps,
  type TagFacetLayout,
} from './TagFacetPanel'
export { TagFacetSheet, type TagFacetSheetProps } from './TagFacetSheet'
