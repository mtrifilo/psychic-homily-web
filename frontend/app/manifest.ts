import type { MetadataRoute } from 'next'

import { SITE_DESCRIPTION } from '@/lib/seo/siteMetadata'

/**
 * Web app manifest, served at /manifest.webmanifest and linked from every
 * page's <head> by Next's metadata file convention.
 *
 * Both colours are the dark page background (.dark --background in
 * app/globals.css), matching the black icon. A manifest holds one value per
 * field; the per-scheme tint comes from the theme-color meta tags built from
 * lib/seo/viewport.ts.
 *
 * The 512 artwork keeps every lit pixel inside the maskable safe zone (the
 * centred circle of radius 40%), so the same file is also listed as maskable.
 */
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: '/',
    name: 'Psychic Homily',
    short_name: 'Psychic Homily',
    description: SITE_DESCRIPTION,
    start_url: '/',
    display: 'standalone',
    background_color: '#0d0805',
    theme_color: '#0d0805',
    icons: [
      { src: '/icons/icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
      { src: '/icons/icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  }
}
