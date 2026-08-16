import type { Viewport } from 'next'

// The app's root viewport, re-exported by app/layout.tsx. It lives here rather
// than inline in the layout so it can be asserted in a unit test: layout.tsx
// calls next/font/local at module scope, which does not resolve under vitest.
export const appViewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: 'white' },
    { media: '(prefers-color-scheme: dark)', color: 'black' },
  ],
  // `viewport-fit=cover` is the single switch that makes every
  // env(safe-area-inset-*) term in this app resolve to the device's reported
  // inset instead of 0. Without it iOS letterboxes the page inside the safe
  // area and all four insets report 0 — which is how the bottom tab bar
  // (PSY-1020) ended up flush against the home indicator, with its sheets, the
  // AppShell padding, and the cookie-banner offset all reserving nothing.
  //
  // Deleting this line does not break a single layout test: it silently
  // reverts every safe-area behaviour in the app at once. That is why it has
  // its own test.
  //
  // It also hands the page the LANDSCAPE notch band, so full-bleed fixed
  // surfaces must inset their own content horizontally — see the AppShell
  // comment for who absorbs what, and PSY-1824 for what is still deferred.
  viewportFit: 'cover',
}
