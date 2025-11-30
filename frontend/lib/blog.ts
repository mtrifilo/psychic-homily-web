import fs from 'fs'
import path from 'path'
import matter from 'gray-matter'
import type { BlogPost, BlogPostMeta, BlogPostFrontmatter } from './types/blog'

// Path to blog content (relative to project root, goes up to parent Hugo project)
const BLOG_CONTENT_PATH = path.join(process.cwd(), '..', 'content', 'blog')

/**
 * Get all blog post slugs for static generation
 */
export function getBlogSlugs(): string[] {
  const files = fs.readdirSync(BLOG_CONTENT_PATH)
  return files
    .filter(file => file.endsWith('.md') && !file.startsWith('_'))
    .map(file => file.replace(/\.md$/, ''))
}

/**
 * Convert Hugo shortcodes to MDX component syntax
 */
function convertShortcodesToMDX(content: string): string {
  // Convert {{< bandcamp album="..." artist="..." title="..." >}}
  // to <Bandcamp album="..." artist="..." title="..." />
  content = content.replace(/\{\{<\s*bandcamp\s+([^>]+)>\}\}/g, (_, attrs) => {
    // Parse attributes and convert to JSX format
    const cleanAttrs = attrs.trim().replace(/\n/g, ' ')
    return `<Bandcamp ${cleanAttrs} />`
  })

  // Note: {{< band "id" >}} shortcodes should be replaced with markdown links
  // in the source files, e.g., [Winter](https://instagram.com/daydreamingwinter)

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
 * Extract a plain text excerpt from markdown content
 */
function extractExcerpt(content: string, maxLength = 200): string {
  // Remove MDX components
  let text = content.replace(/<[^>]+\/>/g, '')
  // Remove markdown formatting
  text = text.replace(/[#*_`\[\]]/g, '')
  // Remove extra whitespace
  text = text.replace(/\s+/g, ' ').trim()
  // Truncate
  if (text.length > maxLength) {
    text = text.substring(0, maxLength).trim() + '...'
  }
  return text
}

/**
 * Get a single blog post by slug
 */
export function getBlogPost(slug: string): BlogPost | null {
  const filePath = path.join(BLOG_CONTENT_PATH, `${slug}.md`)

  if (!fs.existsSync(filePath)) {
    return null
  }

  const fileContents = fs.readFileSync(filePath, 'utf8')
  const { data, content } = matter(fileContents)

  const frontmatter = data as BlogPostFrontmatter
  const mdxContent = convertShortcodesToMDX(content)

  return {
    slug,
    frontmatter,
    content: mdxContent,
    excerpt: extractExcerpt(content),
  }
}

/**
 * Get all blog posts metadata for listing (sorted by date, newest first)
 */
export function getAllBlogPosts(): BlogPostMeta[] {
  const slugs = getBlogSlugs()

  const posts: BlogPostMeta[] = []

  for (const slug of slugs) {
    const post = getBlogPost(slug)
    if (!post) continue

    posts.push({
      slug: post.slug,
      title: post.frontmatter.title,
      date: post.frontmatter.date,
      categories: post.frontmatter.categories || [],
      description: post.frontmatter.description,
      excerpt: post.excerpt,
    })
  }

  // Sort by date, newest first
  posts.sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime())

  return posts
}

/**
 * Get all unique categories from blog posts
 */
export function getAllCategories(): string[] {
  const posts = getAllBlogPosts()
  const categoriesSet = new Set<string>()

  for (const post of posts) {
    for (const category of post.categories) {
      categoriesSet.add(category)
    }
  }

  return Array.from(categoriesSet).sort()
}

/**
 * Get category slug from category name
 */
export function getCategorySlug(category: string): string {
  return category.toLowerCase().replace(/\s+/g, '-')
}

/**
 * Get category name from slug
 */
export function getCategoryFromSlug(slug: string): string | null {
  const categories = getAllCategories()
  return categories.find(cat => getCategorySlug(cat) === slug) || null
}

/**
 * Get all posts for a specific category
 */
export function getPostsByCategory(categorySlug: string): BlogPost[] {
  const categoryName = getCategoryFromSlug(categorySlug)
  if (!categoryName) return []

  const slugs = getBlogSlugs()
  const posts: BlogPost[] = []

  for (const slug of slugs) {
    const post = getBlogPost(slug)
    if (!post) continue

    if (post.frontmatter.categories?.includes(categoryName)) {
      posts.push(post)
    }
  }

  // Sort by date, newest first
  posts.sort(
    (a, b) =>
      new Date(b.frontmatter.date).getTime() -
      new Date(a.frontmatter.date).getTime()
  )

  return posts
}
