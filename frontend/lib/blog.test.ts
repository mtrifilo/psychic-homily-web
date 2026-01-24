import { describe, it, expect, vi, beforeEach } from 'vitest'
import fs from 'fs'
import path from 'path'

// Mock fs module
vi.mock('fs', () => ({
  default: {
    readdirSync: vi.fn(),
    existsSync: vi.fn(),
    readFileSync: vi.fn(),
  },
  readdirSync: vi.fn(),
  existsSync: vi.fn(),
  readFileSync: vi.fn(),
}))

// Mock path.join to return predictable paths
vi.mock('path', async () => {
  const actual = await vi.importActual('path')
  return {
    ...actual,
    default: {
      ...(actual as object),
      join: (...args: string[]) => args.join('/'),
    },
    join: (...args: string[]) => args.join('/'),
  }
})

describe('blog utilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getBlogSlugs', () => {
    it('returns slugs from markdown files', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-01-first-post.md',
        '2025-01-15-second-post.md',
        '2025-02-01-third-post.md',
      ] as unknown as fs.Dirent[])

      const { getBlogSlugs } = await import('./blog')
      const slugs = getBlogSlugs()

      expect(slugs).toEqual([
        '2025-01-01-first-post',
        '2025-01-15-second-post',
        '2025-02-01-third-post',
      ])
    })

    it('filters out non-markdown files', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-01-post.md',
        'readme.txt',
        'image.png',
        '2025-02-01-another.md',
      ] as unknown as fs.Dirent[])

      const { getBlogSlugs } = await import('./blog')
      vi.resetModules()
      const freshModule = await import('./blog')
      const slugs = freshModule.getBlogSlugs()

      expect(slugs).toContain('2025-01-01-post')
      expect(slugs).toContain('2025-02-01-another')
      expect(slugs).not.toContain('readme')
      expect(slugs).not.toContain('image')
    })

    it('filters out files starting with underscore', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '_draft.md',
        '_template.md',
        '2025-01-01-published.md',
      ] as unknown as fs.Dirent[])

      vi.resetModules()
      const { getBlogSlugs } = await import('./blog')
      const slugs = getBlogSlugs()

      expect(slugs).toEqual(['2025-01-01-published'])
    })

    it('returns empty array when no files exist', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([])

      vi.resetModules()
      const { getBlogSlugs } = await import('./blog')
      const slugs = getBlogSlugs()

      expect(slugs).toEqual([])
    })
  })

  describe('getBlogPost', () => {
    it('returns null when file does not exist', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(false)

      vi.resetModules()
      const { getBlogPost } = await import('./blog')
      const post = getBlogPost('non-existent-post')

      expect(post).toBeNull()
    })

    it('parses frontmatter and content correctly', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Test Post"
date: "2025-01-15"
categories:
  - "Technology"
  - "Web Development"
description: "A test post description"
---

# Hello World

This is the content of the post.
`)

      vi.resetModules()
      const { getBlogPost } = await import('./blog')
      const post = getBlogPost('2025-01-15-test-post')

      expect(post).not.toBeNull()
      expect(post?.slug).toBe('2025-01-15-test-post')
      expect(post?.frontmatter.title).toBe('Test Post')
      expect(post?.frontmatter.date).toBe('2025-01-15')
      expect(post?.frontmatter.categories).toEqual([
        'Technology',
        'Web Development',
      ])
      expect(post?.content).toContain('# Hello World')
    })

    it('generates excerpt from content', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Excerpt Test"
date: "2025-02-01"
---

This is the first paragraph with some text that will become the excerpt.
It continues for a bit.
`)

      vi.resetModules()
      const { getBlogPost } = await import('./blog')
      const post = getBlogPost('2025-02-01-excerpt-test')

      expect(post?.excerpt).toBeDefined()
      expect(post?.excerpt).toContain('This is the first paragraph')
    })

    it('converts bandcamp shortcodes to MDX', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Music Post"
date: "2025-03-01"
---

Check out this album:

{{< bandcamp album="123456" artist="Test Artist" title="Album Name" >}}
`)

      vi.resetModules()
      const { getBlogPost } = await import('./blog')
      const post = getBlogPost('2025-03-01-music-post')

      expect(post?.content).toContain('<Bandcamp')
      expect(post?.content).toContain('album="123456"')
      expect(post?.content).not.toContain('{{<')
    })

    it('converts soundcloud shortcodes to MDX', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "SoundCloud Post"
date: "2025-03-15"
---

Listen to this track:

{{< soundcloud url="https://soundcloud.com/artist/track" >}}
`)

      vi.resetModules()
      const { getBlogPost } = await import('./blog')
      const post = getBlogPost('2025-03-15-soundcloud-post')

      expect(post?.content).toContain('<SoundCloud')
      expect(post?.content).toContain('url="https://soundcloud.com/artist/track"')
    })
  })

  describe('getAllBlogPosts', () => {
    it('returns posts sorted by date (newest first)', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-01-old-post.md',
        '2025-03-01-new-post.md',
        '2025-02-01-middle-post.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)

      // Return different content for each file
      vi.mocked(fs.readFileSync).mockImplementation((filePath: fs.PathOrFileDescriptor) => {
        const path = filePath.toString()
        if (path.includes('2025-01-01')) {
          return `---
title: "Old Post"
date: "2025-01-01"
---
Content`
        }
        if (path.includes('2025-02-01')) {
          return `---
title: "Middle Post"
date: "2025-02-01"
---
Content`
        }
        return `---
title: "New Post"
date: "2025-03-01"
---
Content`
      })

      vi.resetModules()
      const { getAllBlogPosts } = await import('./blog')
      const posts = getAllBlogPosts()

      expect(posts.length).toBe(3)
      expect(posts[0].title).toBe('New Post')
      expect(posts[1].title).toBe('Middle Post')
      expect(posts[2].title).toBe('Old Post')
    })

    it('includes metadata for each post', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-15-test.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Test Post"
date: "2025-01-15"
categories:
  - "Category1"
description: "Post description"
---
Post content here.`)

      vi.resetModules()
      const { getAllBlogPosts } = await import('./blog')
      const posts = getAllBlogPosts()

      expect(posts[0]).toMatchObject({
        slug: '2025-01-15-test',
        title: 'Test Post',
        date: '2025-01-15',
        categories: ['Category1'],
        description: 'Post description',
      })
      expect(posts[0].excerpt).toBeDefined()
    })
  })

  describe('getAllCategories', () => {
    it('returns unique categories sorted alphabetically', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'post1.md',
        'post2.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)

      let callCount = 0
      vi.mocked(fs.readFileSync).mockImplementation(() => {
        callCount++
        if (callCount <= 1) {
          return `---
title: "Post 1"
date: "2025-01-01"
categories:
  - "Zebra"
  - "Apple"
---
Content`
        }
        return `---
title: "Post 2"
date: "2025-01-02"
categories:
  - "Banana"
  - "Apple"
---
Content`
      })

      vi.resetModules()
      const { getAllCategories } = await import('./blog')
      const categories = getAllCategories()

      expect(categories).toEqual(['Apple', 'Banana', 'Zebra'])
    })

    it('returns empty array when no posts have categories', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'post.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "No Categories"
date: "2025-01-01"
---
Content`)

      vi.resetModules()
      const { getAllCategories } = await import('./blog')
      const categories = getAllCategories()

      expect(categories).toEqual([])
    })
  })

  describe('getCategorySlug', () => {
    it('converts category name to slug', async () => {
      vi.resetModules()
      const { getCategorySlug } = await import('./blog')

      expect(getCategorySlug('Web Development')).toBe('web-development')
      expect(getCategorySlug('Arizona Artists')).toBe('arizona-artists')
      expect(getCategorySlug('New Release')).toBe('new-release')
    })

    it('handles single word categories', async () => {
      vi.resetModules()
      const { getCategorySlug } = await import('./blog')

      expect(getCategorySlug('Music')).toBe('music')
      expect(getCategorySlug('Technology')).toBe('technology')
    })

    it('handles multiple spaces', async () => {
      vi.resetModules()
      const { getCategorySlug } = await import('./blog')

      // The function uses /\s+/g which replaces whitespace sequences with single dash
      expect(getCategorySlug('Multiple   Spaces')).toBe('multiple-spaces')
    })
  })

  describe('getCategoryFromSlug', () => {
    it('returns category name from slug', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'post.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Test"
date: "2025-01-01"
categories:
  - "Arizona Artists"
  - "New Release"
---
Content`)

      vi.resetModules()
      const { getCategoryFromSlug } = await import('./blog')

      expect(getCategoryFromSlug('arizona-artists')).toBe('Arizona Artists')
      expect(getCategoryFromSlug('new-release')).toBe('New Release')
    })

    it('returns null for non-existent category', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'post.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Test"
date: "2025-01-01"
categories:
  - "Existing Category"
---
Content`)

      vi.resetModules()
      const { getCategoryFromSlug } = await import('./blog')

      expect(getCategoryFromSlug('non-existent-category')).toBeNull()
    })
  })

  describe('getPostsByCategory', () => {
    it('returns posts filtered by category', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'post1.md',
        'post2.md',
        'post3.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)

      let callCount = 0
      vi.mocked(fs.readFileSync).mockImplementation(() => {
        callCount++
        if (callCount === 1 || callCount === 4) {
          return `---
title: "Post in Category"
date: "2025-01-01"
categories:
  - "Target Category"
---
Content`
        }
        if (callCount === 2 || callCount === 5) {
          return `---
title: "Another in Category"
date: "2025-02-01"
categories:
  - "Target Category"
  - "Other"
---
Content`
        }
        return `---
title: "Different Category"
date: "2025-03-01"
categories:
  - "Other"
---
Content`
      })

      vi.resetModules()
      const { getPostsByCategory } = await import('./blog')
      const posts = getPostsByCategory('target-category')

      expect(posts.length).toBe(2)
      expect(posts.every(p => p.frontmatter.categories?.includes('Target Category'))).toBe(true)
    })

    it('returns empty array for non-existent category', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'post.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Test"
date: "2025-01-01"
categories:
  - "Some Category"
---
Content`)

      vi.resetModules()
      const { getPostsByCategory } = await import('./blog')
      const posts = getPostsByCategory('non-existent')

      expect(posts).toEqual([])
    })

    it('returns posts sorted by date (newest first)', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'old.md',
        'new.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)

      let callCount = 0
      vi.mocked(fs.readFileSync).mockImplementation(() => {
        callCount++
        if (callCount === 1 || callCount === 3) {
          return `---
title: "Old Post"
date: "2025-01-01"
categories:
  - "Music"
---
Content`
        }
        return `---
title: "New Post"
date: "2025-03-01"
categories:
  - "Music"
---
Content`
      })

      vi.resetModules()
      const { getPostsByCategory } = await import('./blog')
      const posts = getPostsByCategory('music')

      expect(posts[0].frontmatter.title).toBe('New Post')
      expect(posts[1].frontmatter.title).toBe('Old Post')
    })
  })
})
