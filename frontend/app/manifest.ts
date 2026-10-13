import type { MetadataRoute } from 'next'

import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'
import { PAGE_BACKGROUND } from '@/lib/seo/viewport'

/**
 * Web app manifest, served at /manifest.webmanifest and linked from every
 * page's <head> by Next's metadata file convention.
 *
 * Both colours are the dark page background (PAGE_BACKGROUND.dark); the icon
 * artwork itself sits on pure black. A manifest holds one value per field; the
 * per-scheme browser tint comes from the theme-color meta tags in
 * lib/seo/viewport.ts.
 *
 * /icon.png is app/icon.png (the 192 rel=icon). Both PNGs are rendered by
 * scripts/app-icons.mjs, which refuses to write them unless the 512 keeps every
 * lit pixel inside the maskable safe zone, so that file is also listed as
 * maskable.
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: '/',
    name: 'Psychic Homily',
    short_name: 'Psychic Homily',
    description: SITE_DESCRIPTION,
    start_url: '/',
    display: 'standalone',
    background_color: PAGE_BACKGROUND.dark,
    theme_color: PAGE_BACKGROUND.dark,
    icons: [
      { src: '/icon.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  }
}
