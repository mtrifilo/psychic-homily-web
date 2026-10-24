import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { extname, join, relative, sep } from 'node:path'

/**
 * Which surfaces render MobileGraphTeaser as their sub-640px graph form.
 *
 * What a graph section shows below the canvas gate is a design decision made
 * per surface, so adding or dropping the teaser on one is a deliberate change
 * that should come with its design: this inventory makes it show up in review
 * instead of happening as a side effect of an edit to a shared parent. When
 * such a change is intended, update the list.
 *
 * A source scan rather than a render test, so it covers every file and not
 * only the surfaces a test happens to mount. The artist page's own placement
 * (its phone layout shows the Similar artists list under the shows and does
 * not mount the section) is pinned by ArtistDetail.test; ArtistConnectionsSection
 * stays in this list because outside the phone layout a column measuring under 640px
 * still gets the teaser.
 */

const FRONTEND_ROOT = join(import.meta.dirname, '..', '..')
const SCANNED_ROOTS = ['app', 'components', 'features']
const TEASER_RENDER = /<MobileGraphTeaser\b/

const EXPECTED_SURFACES = [
  'features/artists/components/ArtistConnectionsSection.tsx',
  'features/collections/components/CollectionGraph.tsx',
  'features/home/components/HomeSceneGraph.tsx',
  'features/radio/components/StationGraph.tsx',
  'features/scenes/components/SceneGraph.tsx',
  'features/venues/components/VenueBillNetwork.tsx',
]

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    const isSource = extname(entry.name) === '.tsx'
    const isTest = /\.test\.tsx$/.test(entry.name)
    return isSource && !isTest ? [path] : []
  })
}

describe('MobileGraphTeaser surfaces', () => {
  it('is rendered by exactly the expected surfaces', () => {
    const rendering = SCANNED_ROOTS.flatMap(root =>
      sourceFiles(join(FRONTEND_ROOT, root))
    )
      .filter(path => TEASER_RENDER.test(readFileSync(path, 'utf8')))
      .map(path => relative(FRONTEND_ROOT, path).split(sep).join('/'))
      .sort()

    expect(rendering).toEqual(EXPECTED_SURFACES)
  })
})
