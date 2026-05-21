import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { BlogPost } from '@/features/blog'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

// The blog page sources posts synchronously from the filesystem-backed blog
// feature (not fetch). Mock the feature so generateMetadata reads a controlled
// post payload and the heavy MDX render path is stubbed out.
const getBlogPostMock = vi.fn()

vi.mock('@/features/blog', () => ({
  getBlogPost: (slug: string) => getBlogPostMock(slug),
  getBlogSlugs: vi.fn(() => []),
  MDXContent: () => null,
}))

import { generateMetadata } from './page'

function buildPost(overrides: Partial<BlogPost> = {}): BlogPost {
  return {
    slug: '2026-01-15-test-post',
    frontmatter: {
      title: 'Test Post',
      date: '2026-01-15',
      description: 'A test post description',
      categories: ['Technology'],
    },
    content: '# Hello World',
    excerpt: 'An auto-generated excerpt.',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('blog/[slug] generateMetadata', () => {
  it('uses the frontmatter title as the title when the post is found', async () => {
    getBlogPostMock.mockReturnValueOnce(buildPost())

    const meta = await generateMetadata({ params: Promise.resolve({ slug: '2026-01-15-test-post' }) })

    expect(meta.title).toBe('Test Post')
  })

  it('uses the frontmatter description when present', async () => {
    getBlogPostMock.mockReturnValueOnce(buildPost())

    const meta = await generateMetadata({ params: Promise.resolve({ slug: '2026-01-15-test-post' }) })

    expect(meta.description).toBe('A test post description')
  })

  it('falls back to the excerpt when frontmatter has no description', async () => {
    getBlogPostMock.mockReturnValueOnce(
      buildPost({
        frontmatter: { title: 'No Description Post', date: '2026-01-15' },
        excerpt: 'An auto-generated excerpt.',
      })
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: '2026-01-15-test-post' }) })

    expect(meta.description).toBe('An auto-generated excerpt.')
  })

  it('sets the canonical URL to https://psychichomily.com/blog/{slug}', async () => {
    getBlogPostMock.mockReturnValueOnce(buildPost())

    const meta = await generateMetadata({ params: Promise.resolve({ slug: '2026-01-15-test-post' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/blog/2026-01-15-test-post'
    )
  })

  it('sets openGraph title/description/url and the article type with publishedTime', async () => {
    getBlogPostMock.mockReturnValueOnce(buildPost())

    const meta = await generateMetadata({ params: Promise.resolve({ slug: '2026-01-15-test-post' }) })

    expect(meta.openGraph?.title).toBe('Test Post')
    expect(meta.openGraph?.description).toBe('A test post description')
    expect(meta.openGraph?.url).toBe('/blog/2026-01-15-test-post')
    // Blog posts use the OpenGraph "article" type (not "website").
    expect((meta.openGraph as { type?: string })?.type).toBe('article')
    expect((meta.openGraph as { publishedTime?: string })?.publishedTime).toBe('2026-01-15')
  })

  it('falls back to the "Post Not Found" title when the post is missing', async () => {
    getBlogPostMock.mockReturnValueOnce(null)

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Post Not Found')
    // No canonical alternate or openGraph on the fallback shape.
    expect(meta.alternates).toBeUndefined()
    expect(meta.openGraph).toBeUndefined()
  })
})
