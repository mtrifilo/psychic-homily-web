// Public API for the home feature module (PSY-389 logged-out discovery
// landing). The page variants and the signed-in sections (AnonymousHome,
// SignedInHome, SavedShowsModule, NearbyShowsSection) are deliberately NOT
// exported here: app/_components/HomeContentSlot.tsx deep-imports them, and
// features/sharedChunkBarrelGuard.test.ts pins their absence.

export { HomeHero } from './components/HomeHero'
export { CommunityPulseBand } from './components/CommunityPulseBand'
export { LatestRadioShows } from './components/LatestRadioShows'
export { HomeSceneGraph } from './components/HomeSceneGraph'
