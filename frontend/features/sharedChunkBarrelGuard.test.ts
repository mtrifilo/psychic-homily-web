import { describe, it, expect } from 'vitest'

/**
 * PSY-1772 regression gate.
 *
 * Turbopack does not tree-shake `'use client'` barrels per-export, so merely
 * LISTING a component in a barrel that is reachable from `app/layout.tsx` puts
 * that component in the one client chunk every route loads eagerly — nobody has
 * to import the name for it to cost bytes.
 *
 * The eviction those components got is therefore held in place by an ABSENCE,
 * and an absence is invisible: re-adding `export { ShowDetail } from
 * './ShowDetail'` typechecks, passes every other test, and silently undoes the
 * work. `next build` is not run in CI, so nothing else would notice.
 *
 * This file makes the absence assertable. If it fails, do not "fix" it by
 * loosening the assertion — either import the component from its file (the
 * route pages do this via `dynamic()`), or re-measure the shared chunk and
 * update the ticket.
 *
 * Deliberately NOT asserted here: the sibling detail components in
 * features/{venues,labels,festivals,requests}. Those are barrel-exported today
 * and were measured to be outside the global chunk (their barrels are not
 * root-layout-reachable), so an assertion would be describing luck rather than
 * an invariant. See the PSY-1772 PR for the follow-up.
 */

const EVICTED: ReadonlyArray<readonly [string, () => Promise<object>, readonly string[]]> = [
  ['@/features/shows', () => import('@/features/shows'), ['ShowDetail']],
  ['@/features/shows/components', () => import('@/features/shows/components'), ['ShowDetail']],
  ['@/features/tags', () => import('@/features/tags'), ['TagDetail']],
  ['@/features/tags/components', () => import('@/features/tags/components'), ['TagDetail']],
  ['@/features/scenes', () => import('@/features/scenes'), ['SceneDetailView']],
  [
    '@/features/scenes/components',
    () => import('@/features/scenes/components'),
    // SceneCalendar joins the eviction list (PSY-1783): SceneDetail deep-imports
    // it, and listing it here would put four weeks of calendar rendering into
    // the chunk every route loads.
    //
    // SceneRooms / SceneNewBands / SceneRoster join it for the same reason
    // (PSY-1784). They are named HERE and not only in the barrel's own comment
    // because a comment does not fail a build: without these entries a future
    // `export { SceneRoster }` typechecks, passes every test, and ships the
    // whole roster surface into the global chunk silently.
    [
      'SceneDetailView',
      'SceneGraph',
      'SceneCalendar',
      'SceneRooms',
      'SceneNewBands',
      'SceneRoster',
    ],
  ],
  [
    '@/features/releases/components',
    () => import('@/features/releases/components'),
    ['ReleaseDetail'],
  ],
]

describe('shared-chunk barrel guard (PSY-1772)', () => {
  // These barrels pull a whole feature's client surface, so evaluating one can
  // exceed the 5s default. That cost is the very thing being guarded against.
  const BARREL_LOAD_TIMEOUT_MS = 30_000

  for (const [name, load, forbidden] of EVICTED) {
    it(
      `${name} does not re-export ${forbidden.join(' / ')}`,
      async () => {
        const barrel = await load()
        for (const forbiddenName of forbidden) {
          expect(Object.keys(barrel)).not.toContain(forbiddenName)
        }
      },
      BARREL_LOAD_TIMEOUT_MS,
    )
  }

  it('the evicted components are still importable from their own files', async () => {
    const [shows, tags, scenes, sceneGraph, releases] = await Promise.all([
      import('@/features/shows/components/ShowDetail'),
      import('@/features/tags/components/TagDetail'),
      import('@/features/scenes/components/SceneDetail'),
      import('@/features/scenes/components/SceneGraph'),
      import('@/features/releases/components/ReleaseDetail'),
    ])
    expect(typeof shows.ShowDetail).toBe('function')
    expect(typeof tags.TagDetail).toBe('function')
    expect(typeof scenes.SceneDetailView).toBe('function')
    expect(typeof sceneGraph.SceneGraph).toBe('function')
    expect(typeof releases.ReleaseDetail).toBe('function')
  })
})
