import { Suspense } from 'react'
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
//
// The boundary is what keeps that shell prerenderable now that the island reads
// `?city=` (atlasCityEntry.ts): a client component reading search params has to
// sit under one, or the whole route renders on request.
export default function AtlasPage() {
  return (
    <Suspense fallback={null}>
      <AtlasGlobe />
    </Suspense>
  )
}
