'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Star } from 'lucide-react'
import { useMyFollowing } from '@/lib/hooks/common/useFollow'
import { compareScenesByActivity } from './globeScale'
import { isPlaceableScene, type PlaceableScene } from './globeTypes'
import type { SceneListItem } from '../types'

/**
 * "My Scenes" quick-return strip (PSY-1340): the viewer's followed scenes as
 * chips over the globe — the retention path back to the places they care
 * about. A placeable pick flies the camera + opens the preview (same seam as
 * search/Drift); an unplaceable one navigates to its scene page. Renders
 * nothing while logged out or with no follows (useMyFollowing is auth-gated),
 * so the surface costs nothing until it has something to say.
 */
// Visual cap: the strip is an overlay on the globe, not a management surface —
// show the liveliest few and defer the rest to /library?tab=scenes via the
// "+N more" chip (which also surfaces the fetch cap below, so no follow is
// ever silently invisible).
const STRIP_CHIP_LIMIT = 8
// Fetch cap shared with AtlasGlobe's dot-tint query (same key → one request).
export const MY_SCENES_FETCH_LIMIT = 100

export function MyScenesStrip({
  scenes,
  onPick,
}: {
  /** ALL scenes — follows are matched to them by slug. */
  scenes: SceneListItem[]
  onPick: (scene: PlaceableScene) => void
}) {
  const router = useRouter()
  const { data } = useMyFollowing({ type: 'scene', limit: MY_SCENES_FETCH_LIMIT })

  // Liveliest-first, like every other scene ordering surface. A follow whose
  // slug isn't in the scenes list (below the listing threshold this season)
  // still renders — it navigates to the scene page instead of flying.
  const followed = useMemo(() => {
    const bySlug = new Map(scenes.map((s) => [s.slug, s]))
    const rows = (data?.following ?? []).map((f) => ({
      slug: f.slug,
      name: f.name,
      scene: bySlug.get(f.slug) ?? null,
    }))
    return rows.sort((a, b) => {
      if (a.scene && b.scene) return compareScenesByActivity(a.scene, b.scene)
      if (a.scene) return -1
      if (b.scene) return 1
      return a.name.localeCompare(b.name)
    })
  }, [data, scenes])

  if (followed.length === 0) return null

  const shown = followed.slice(0, STRIP_CHIP_LIMIT)
  // Count against the response TOTAL, not the fetched page — a truncated
  // fetch (more follows than MY_SCENES_FETCH_LIMIT) still surfaces here
  // instead of silently vanishing.
  const more = Math.max(0, (data?.total ?? followed.length) - shown.length)

  return (
    // pointer-events-none on the shell: the wrap box's gaps and trailing row
    // space must not swallow globe drags/hovers — only the chips themselves
    // are interactive. Width stays clear of the right-side preview panel
    // (max-w-sm) down to the 640px mobile gate.
    <nav
      aria-label="My scenes"
      className="pointer-events-none absolute left-4 top-16 z-10 flex max-w-[min(50vw,24rem)] flex-wrap items-center gap-1.5"
    >
      <Star aria-hidden className="h-3.5 w-3.5 text-primary" />
      {shown.map(({ slug, name, scene }) => (
        <button
          key={slug}
          type="button"
          onClick={() => {
            if (scene && isPlaceableScene(scene)) {
              onPick(scene)
            } else {
              router.push(`/scenes/${slug}`)
            }
          }}
          className="pointer-events-auto rounded-full border border-border bg-background/90 px-2.5 py-1 text-xs text-muted-foreground backdrop-blur transition-colors hover:border-primary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {name}
        </button>
      ))}
      {more > 0 && (
        <Link
          href="/library?tab=scenes"
          className="pointer-events-auto rounded-full border border-border bg-background/90 px-2.5 py-1 text-xs text-muted-foreground backdrop-blur transition-colors hover:border-primary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          +{more} more
        </Link>
      )}
    </nav>
  )
}
