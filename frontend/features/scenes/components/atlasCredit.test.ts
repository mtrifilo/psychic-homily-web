import { describe, it, expect } from 'vitest'
import { atlasTopCreditShown } from './atlasCredit'

describe('atlasTopCreditShown', () => {
  function pane(html: string): HTMLElement {
    const root = document.createElement('div')
    root.innerHTML = html
    return root
  }

  it('is true for a top-left credit with something to credit', () => {
    expect(
      atlasTopCreditShown(
        pane(
          '<div data-atlas-credit="top"><details class="maplibregl-ctrl maplibregl-ctrl-attrib">OpenStreetMap</details></div>',
        ),
      ),
    ).toBe(true)
  })

  it('is false while MapLibre hides an empty credit', () => {
    expect(
      atlasTopCreditShown(
        pane(
          '<div data-atlas-credit="top"><details class="maplibregl-ctrl maplibregl-ctrl-attrib maplibregl-attrib-empty"></details></div>',
        ),
      ),
    ).toBe(false)
  })

  it('is false with no credit control, as before the map mounts its controls', () => {
    expect(atlasTopCreditShown(pane('<div data-atlas-credit="top"></div>'))).toBe(
      false,
    )
  })

  it('ignores a credit outside the top-left placement', () => {
    expect(
      atlasTopCreditShown(
        pane(
          '<div><details class="maplibregl-ctrl maplibregl-ctrl-attrib">OpenStreetMap</details></div>',
        ),
      ),
    ).toBe(false)
  })
})
