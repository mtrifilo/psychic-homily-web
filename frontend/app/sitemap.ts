import { MetadataRoute } from 'next'
import { getBlogSlugs } from '@/lib/blog'
import { getMixSlugs } from '@/lib/mixes'

const BASE_URL = 'https://psychichomily.com'
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'

interface ShowResponse {
  slug?: string
  updated_at?: string
}

interface VenueResponse {
  slug?: string
  updated_at?: string
}

interface ArtistResponse {
  slug?: string
  updated_at?: string
}

async function fetchShows(): Promise<ShowResponse[]> {
  try {
    const res = await fetch(`${API_BASE_URL}/shows`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
  } catch {
    console.error('Failed to fetch shows for sitemap')
  }
  return []
}

async function fetchVenues(): Promise<{ venues: VenueResponse[] }> {
  try {
    const res = await fetch(`${API_BASE_URL}/venues`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
  } catch {
    console.error('Failed to fetch venues for sitemap')
  }
  return { venues: [] }
}

async function fetchArtists(): Promise<{ artists: ArtistResponse[] }> {
  try {
    const res = await fetch(`${API_BASE_URL}/artists`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
  } catch {
    console.error('Failed to fetch artists for sitemap')
  }
  return { artists: [] }
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  // Fetch dynamic content in parallel
  const [shows, venuesData, artistsData] = await Promise.all([
    fetchShows(),
    fetchVenues(),
    fetchArtists(),
  ])

  // Static pages
  const staticPages: MetadataRoute.Sitemap = [
    {
      url: BASE_URL,
      lastModified: new Date(),
      changeFrequency: 'daily',
      priority: 1,
    },
    {
      url: `${BASE_URL}/shows`,
      lastModified: new Date(),
      changeFrequency: 'daily',
      priority: 0.9,
    },
    {
      url: `${BASE_URL}/venues`,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 0.8,
    },
    {
      url: `${BASE_URL}/blog`,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 0.8,
    },
    {
      url: `${BASE_URL}/dj-sets`,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 0.7,
    },
    {
      url: `${BASE_URL}/privacy`,
      lastModified: new Date(),
      changeFrequency: 'monthly',
      priority: 0.3,
    },
    {
      url: `${BASE_URL}/terms`,
      lastModified: new Date(),
      changeFrequency: 'monthly',
      priority: 0.3,
    },
  ]

  // Dynamic show pages
  const showPages: MetadataRoute.Sitemap = shows
    .filter(show => show.slug)
    .map(show => ({
      url: `${BASE_URL}/shows/${show.slug}`,
      lastModified: show.updated_at ? new Date(show.updated_at) : new Date(),
      changeFrequency: 'weekly' as const,
      priority: 0.8,
    }))

  // Dynamic venue pages
  const venuePages: MetadataRoute.Sitemap = (venuesData.venues || [])
    .filter(venue => venue.slug)
    .map(venue => ({
      url: `${BASE_URL}/venues/${venue.slug}`,
      lastModified: venue.updated_at ? new Date(venue.updated_at) : new Date(),
      changeFrequency: 'monthly' as const,
      priority: 0.6,
    }))

  // Dynamic artist pages
  const artistPages: MetadataRoute.Sitemap = (artistsData.artists || [])
    .filter(artist => artist.slug)
    .map(artist => ({
      url: `${BASE_URL}/artists/${artist.slug}`,
      lastModified: artist.updated_at ? new Date(artist.updated_at) : new Date(),
      changeFrequency: 'monthly' as const,
      priority: 0.6,
    }))

  // Dynamic blog posts
  const blogSlugs = getBlogSlugs()
  const blogPages: MetadataRoute.Sitemap = blogSlugs.map(slug => ({
    url: `${BASE_URL}/blog/${slug}`,
    lastModified: new Date(),
    changeFrequency: 'monthly' as const,
    priority: 0.6,
  }))

  // Dynamic DJ sets
  const mixSlugs = getMixSlugs()
  const mixPages: MetadataRoute.Sitemap = mixSlugs.map(slug => ({
    url: `${BASE_URL}/dj-sets/${slug}`,
    lastModified: new Date(),
    changeFrequency: 'monthly' as const,
    priority: 0.5,
  }))

  return [
    ...staticPages,
    ...showPages,
    ...venuePages,
    ...artistPages,
    ...blogPages,
    ...mixPages,
  ]
}
