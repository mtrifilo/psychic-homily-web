import { Metadata } from 'next'
import PlaylistsFeedPage from './_components/PlaylistsFeedPage'

export const metadata: Metadata = {
  title: 'All playlists — Radio',
  description:
    'Every playlist tracked across the dial, newest first — independent radio wired into the knowledge graph',
  openGraph: {
    title: 'All playlists — Radio',
    description:
      'Every playlist tracked across the dial, newest first — independent radio wired into the knowledge graph',
    type: 'website',
    url: '/radio/playlists',
  },
}

export default function RadioPlaylistsPage() {
  return <PlaylistsFeedPage />
}
