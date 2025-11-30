import fs from 'fs'
import path from 'path'
import matter from 'gray-matter'
import type { Mix, MixMeta, MixFrontmatter } from './types/mix'

// Path to mixes content (relative to project root, goes up to parent Hugo project)
const MIXES_CONTENT_PATH = path.join(process.cwd(), '..', 'content', 'mixes')

/**
 * Get all mix slugs for static generation
 */
export function getMixSlugs(): string[] {
  const files = fs.readdirSync(MIXES_CONTENT_PATH)
  return files
    .filter(file => file.endsWith('.md') && !file.startsWith('_'))
    .map(file => file.replace(/\.md$/, ''))
}

/**
 * Convert Hugo shortcodes to MDX component syntax
 */
function convertShortcodesToMDX(content: string): string {
  // Convert {{< soundcloud url="..." ... >}}
  content = content.replace(
    /\{\{<\s*soundcloud\s+([^>]+)>\}\}/g,
    (_, attrs) => {
      const cleanAttrs = attrs.trim().replace(/\n/g, ' ')
      return `<SoundCloud ${cleanAttrs} />`
    }
  )

  return content
}

/**
 * Get a single mix by slug
 */
export function getMix(slug: string): Mix | null {
  const filePath = path.join(MIXES_CONTENT_PATH, `${slug}.md`)

  if (!fs.existsSync(filePath)) {
    return null
  }

  const fileContents = fs.readFileSync(filePath, 'utf8')
  const { data, content } = matter(fileContents)

  const frontmatter = data as MixFrontmatter
  const mdxContent = convertShortcodesToMDX(content)

  return {
    slug,
    frontmatter,
    content: mdxContent,
  }
}

/**
 * Get all mixes metadata for listing (sorted by date, newest first)
 */
export function getAllMixes(): MixMeta[] {
  const slugs = getMixSlugs()
  const mixes: MixMeta[] = []

  for (const slug of slugs) {
    const mix = getMix(slug)
    if (!mix) continue

    mixes.push({
      slug: mix.slug,
      title: mix.frontmatter.title,
      date: mix.frontmatter.date,
      description: mix.frontmatter.description,
      artist: mix.frontmatter.artist,
      soundcloud_url: mix.frontmatter.soundcloud_url,
      artist_url: mix.frontmatter.artist_url,
      track_url: mix.frontmatter.track_url,
    })
  }

  // Sort by date, newest first
  mixes.sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime())

  return mixes
}

