export { CalendarFeedSection } from './CalendarFeedSection'
export { CollectionCard } from './CollectionCard'
// CollectionDetail is intentionally NOT re-exported here (PSY-951 / PSY-944).
// A `'use client'` barrel is not reliably tree-shaken per-export under
// Turbopack, so re-exporting CollectionDetail would drag it — and the
// dynamic-import boundaries it sets up for `@dnd-kit` / `marked` / `dompurify` —
// into every route that imports any sibling from this barrel (e.g.
// `/collections` imports CollectionList), keeping it multi-route-reachable and
// re-hoisting those libs into the global shared chunk. The only consumer is
// `app/collections/[slug]/page.tsx`, which imports it from the file directly.
export { CollectionList } from './CollectionList'
export { EntityCollections } from './EntityCollections'
export { UserCollections } from './UserCollections'
