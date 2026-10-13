import type { MetadataRoute } from 'next'

import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'
import { PAGE_BACKGROUND } from '@/lib/seo/viewport'

/**
 * Web app manifest, served at /manifest.webmanifest and linked from every
 * page's <head> by Next's metadata file convention.
 *
 * Both colours are the dark page background, matching the black icon. A
 * manifest holds one value per field; the per-scheme tint comes from the
 * theme-color meta tags in lib/seo/viewport.ts.
 *
 * /icon.png is app/icon.png (the 192 rel=icon). The 512 artwork keeps every
 * lit pixel inside the maskable safe zone (the centred circle of radius 40%),
 * so the same file is also listed as maskable.
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
