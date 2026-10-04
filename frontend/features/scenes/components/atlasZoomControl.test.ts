import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, it, expect } from 'vitest'

/**
 * The city view's zoom control is MapLibre's NavigationControl, built
 * imperatively, so its button size lives in app/globals.css rather than in a
 * class this suite could read off a rendered node. AtlasGlobe marks the
 * sheet layout's map pane with `data-atlas-layout="sheet"`; the rule keyed
 * on it is what makes the buttons 36px there.
 */
describe('Atlas zoom control size', () => {
  const css = readFileSync(resolve(__dirname, '../../../app/globals.css'), 'utf8')

  it('gives the zoom buttons 36px targets in the sheet layout', () => {
    const rule = css.match(
      /\[data-atlas-layout='sheet'\]\s+\.maplibregl-map\s+\.maplibregl-ctrl-group\s+button\s*\{([^}]*)\}/,
    )
    expect(rule).not.toBeNull()
    const body = rule![1]
    expect(body).toMatch(/\bwidth:\s*36px;/)
    expect(body).toMatch(/\bheight:\s*36px;/)
  })
})
