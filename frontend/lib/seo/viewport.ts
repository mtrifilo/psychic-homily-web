import type { Viewport } from 'next'

/**
 * The page background of each colour scheme, as declared by `--background` in
 * app/globals.css (the light :root block and the .dark block), for the
 * metadata that names it: the theme-color tags below and app/manifest.ts.
 * Metadata cannot read CSS custom properties, so viewport.test.ts asserts
 * these values match the stylesheet.
 */
export const PAGE_BACKGROUND = { light: '#f4f1ea', dark: '#0d0805' } as const

// The app's root viewport, re-exported by app/layout.tsx. It lives here rather
// than inline in the layout so it can be asserted in a unit test: layout.tsx
// calls next/font/local at module scope, which does not resolve under vitest.
export const appViewport: Viewport = {
  // Selected by the OS colour scheme (the media query), not by the in-app
  // theme choice.
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: PAGE_BACKGROUND.light },
    { media: '(prefers-color-scheme: dark)', color: PAGE_BACKGROUND.dark },
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
