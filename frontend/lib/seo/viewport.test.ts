import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, it, expect } from 'vitest'
import { appViewport, PAGE_BACKGROUND } from './viewport'

describe('appViewport', () => {
  // Every env(safe-area-inset-*) term in the app is inert without this: iOS
  // letterboxes the page inside the safe area and all four insets report 0.
  // Nothing else in the suite fails if it is removed — the bar's height, the
  // shell's padding, and the cookie-banner offset all still compute, they just
  // compute against zero — so this is the assertion standing between a
  // one-word edit and silently un-shipping PSY-1820 on every device.
  it('sets viewport-fit=cover, which every safe-area inset depends on', () => {
    expect(appViewport.viewportFit).toBe('cover')
  })

  // Kept alongside so a rewrite of this object cannot quietly drop the
  // light/dark theme colors while satisfying the assertion above.
  it("tints the browser chrome with each scheme's page background", () => {
    expect(appViewport.themeColor).toEqual([
      { media: '(prefers-color-scheme: light)', color: PAGE_BACKGROUND.light },
      { media: '(prefers-color-scheme: dark)', color: PAGE_BACKGROUND.dark },
    ])
  })
})

describe('PAGE_BACKGROUND', () => {
  // Literal copies, so a palette change is re-approved for the browser tint and
  // the web manifest rather than followed silently.
  it('holds the approved light and dark page backgrounds', () => {
    expect(PAGE_BACKGROUND).toEqual({ light: '#f4f1ea', dark: '#0d0805' })
  })

  it('matches --background in the light and .dark blocks of app/globals.css', () => {
    const css = readFileSync(resolve(process.cwd(), 'app/globals.css'), 'utf8')
    // `.dark {` at line start: earlier `.dark` mentions (the custom variant,
    // prose in comments) are not the token block.
    const darkStart = css.search(/^\.dark \{/m)
    expect(darkStart).toBeGreaterThan(0)
    expect(css.slice(0, darkStart)).toContain(`--background: ${PAGE_BACKGROUND.light};`)
    expect(css.slice(darkStart, css.indexOf('\n}', darkStart))).toContain(
      `--background: ${PAGE_BACKGROUND.dark};`
    )
  })
})
