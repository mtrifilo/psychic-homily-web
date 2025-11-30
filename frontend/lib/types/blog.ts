/**
 * Blog-related TypeScript types
 */

export interface BlogPostFrontmatter {
  title: string
  date: string
  categories?: string[]
  description?: string
}

export interface BlogPost {
  slug: string
  frontmatter: BlogPostFrontmatter
  content: string
  excerpt: string
}

export interface BlogPostMeta {
  slug: string
  title: string
  date: string
  categories: string[]
  description?: string
  excerpt: string
}

