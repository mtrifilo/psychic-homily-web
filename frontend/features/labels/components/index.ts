export { LabelCard } from './LabelCard'
// LabelDetail stays barrel-exported, unlike its shows/tags/scenes/releases peers
// (PSY-1772): this barrel is NOT reachable from `app/layout.tsx`, so it was
// measured to be outside the global shared chunk. It is not free, though — the
// /labels LIST route imports LabelList from here and gets LabelDetail with it.
// See the PSY-1772 PR for the follow-up.
export { LabelDetail } from './LabelDetail'
export { LabelList } from './LabelList'
export { LabelSearch } from './LabelSearch'
