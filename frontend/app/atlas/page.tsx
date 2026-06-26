import type { Metadata } from 'next'
import { AtlasGlobe } from '@/features/scenes/components'

export const metadata: Metadata = {
  title: 'Atlas — Psychic Homily',
  description:
    'Spin the globe to discover live-music scenes city by city — shows, venues, and the artists who play them.',
}

// The page IS the globe (PSY-1213). AtlasGlobe is a client island that lazy-loads
// the WebGL canvas (ssr:false) and fetches scene data + visitor geo client-side,
// so this route's static shell stays light.
export default function AtlasPage() {
  return <AtlasGlobe />
}
