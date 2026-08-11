export { FestivalCard } from './FestivalCard'
// FestivalDetail stays barrel-exported, unlike its shows/tags/scenes/releases
// peers (PSY-1772): this barrel is NOT reachable from `app/layout.tsx`, so it
// was measured to be outside the global shared chunk. Keep it that way — if this
// barrel ever gains a root-reachable importer, apply the recipe documented in
// features/scenes/components/index.ts.
export { FestivalDetail } from './FestivalDetail'
export { FestivalLineup } from './FestivalLineup'
export { FestivalList } from './FestivalList'
export { SimilarFestivals } from './SimilarFestivals'
export { ArtistTrajectoryChart } from './ArtistTrajectoryChart'
export { SeriesHistory } from './SeriesHistory'
