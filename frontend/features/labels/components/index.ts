export { LabelCard } from './LabelCard'
// LabelDetail stays barrel-exported, unlike its shows/tags/scenes/releases peers:
// this barrel is not reachable from `app/layout.tsx`, so listing it costs only
// the routes that import this barrel. The /labels list route imports LabelList
// from its own file, so it does not pay for LabelDetail.
export { LabelDetail } from './LabelDetail'
export { LabelList } from './LabelList'
export { LabelSearch } from './LabelSearch'
