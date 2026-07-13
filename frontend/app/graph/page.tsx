import type { Metadata } from 'next'

import { GraphObservatory } from '@/features/graph'

export const metadata: Metadata = {
  title: 'Graph Observatory',
  description: 'Search artists, inspect their connections, and follow a trail through the Psychic Homily knowledge graph.',
  alternates: { canonical: 'https://psychichomily.com/graph' },
  openGraph: {
    title: 'Graph Observatory | Psychic Homily',
    description: 'Search artists, inspect their connections, and follow a trail through the Psychic Homily knowledge graph.',
    url: '/graph',
    type: 'website',
  },
}

export default function GraphPage() {
  return <GraphObservatory />
}
