/**
 * Mix/DJ Set related TypeScript types
 */

export interface MixFrontmatter {
  title: string
  date: string
  description?: string
  artist: string
  soundcloud_url: string
  artist_url?: string
  track_url?: string
}

export interface Mix {
  slug: string
  frontmatter: MixFrontmatter
  content: string
}

export interface MixMeta {
  slug: string
  title: string
  date: string
  description?: string
  artist: string
  soundcloud_url: string
  artist_url?: string
  track_url?: string
}

