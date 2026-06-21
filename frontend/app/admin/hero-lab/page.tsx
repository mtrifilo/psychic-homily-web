import type { Metadata } from 'next'
import { HeroLab } from './_components/HeroLab'

// Internal design-exploration surface — keep it out of search indexes.
export const metadata: Metadata = {
  title: 'Hero Lab',
  robots: { index: false, follow: false },
}

export default function HeroLabPage() {
  return <HeroLab />
}
