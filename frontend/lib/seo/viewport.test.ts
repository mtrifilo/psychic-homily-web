import { describe, it, expect } from 'vitest'
import { appViewport } from './viewport'

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
  it('keeps the light and dark theme colors', () => {
    expect(appViewport.themeColor).toEqual([
      { media: '(prefers-color-scheme: light)', color: 'white' },
      { media: '(prefers-color-scheme: dark)', color: 'black' },
    ])
  })
})
