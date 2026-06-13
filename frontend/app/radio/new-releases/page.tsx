import { Metadata } from 'next'
import NewReleaseRadarPage from './_components/NewReleaseRadarPage'

export const metadata: Metadata = {
  title: 'New release radar — Radio',
  description:
    'New releases surfaced by radio play across the dial — independent radio wired into the knowledge graph',
  openGraph: {
    title: 'New release radar — Radio',
    description:
      'New releases surfaced by radio play across the dial — independent radio wired into the knowledge graph',
    type: 'website',
    url: '/radio/new-releases',
  },
}

export default function RadioNewReleasesPage() {
  return <NewReleaseRadarPage />
}
