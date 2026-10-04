'use client'

import { preloadAtlasMapAtPageLoad } from '@/features/scenes/components/atlasMapPreload'

// Runs when this module first evaluates, which is while the /atlas page is
// still hydrating, well before AtlasGlobe mounts and measures its container.
// Only app/atlas/page.tsx imports this file, so no other route evaluates it.
preloadAtlasMapAtPageLoad()

/** Renders nothing; importing it on /atlas starts the map's downloads early. */
export function AtlasMapPreloader() {
  return null
}
