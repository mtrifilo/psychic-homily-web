import { Metadata } from 'next'
import RadioHub from './_components/RadioHub'

export const metadata: Metadata = {
  title: 'Radio',
  description: 'Explore radio stations, shows, and playlists tracked on Psychic Homily',
  openGraph: {
    title: 'Radio',
    description: 'Explore radio stations, shows, and playlists tracked on Psychic Homily',
    type: 'website',
    url: '/radio',
  },
}

export default function RadioPage() {
  return <RadioHub />
}
