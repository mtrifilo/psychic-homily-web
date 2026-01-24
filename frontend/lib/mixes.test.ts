import { describe, it, expect, vi, beforeEach } from 'vitest'
import fs from 'fs'

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

describe('mixes utilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('getMixSlugs', () => {
    it('returns slugs from markdown files', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-01-mix-one.md',
        '2025-02-15-mix-two.md',
        '2025-03-01-mix-three.md',
      ] as unknown as fs.Dirent[])

      vi.resetModules()
      const { getMixSlugs } = await import('./mixes')
      const slugs = getMixSlugs()

      expect(slugs).toEqual([
        '2025-01-01-mix-one',
        '2025-02-15-mix-two',
        '2025-03-01-mix-three',
      ])
    })

    it('filters out non-markdown files', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-01-mix.md',
        'cover.jpg',
        'tracklist.txt',
        '2025-02-01-another-mix.md',
      ] as unknown as fs.Dirent[])

      vi.resetModules()
      const { getMixSlugs } = await import('./mixes')
      const slugs = getMixSlugs()

      expect(slugs).toContain('2025-01-01-mix')
      expect(slugs).toContain('2025-02-01-another-mix')
      expect(slugs).not.toContain('cover')
      expect(slugs).not.toContain('tracklist')
    })

    it('filters out files starting with underscore', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '_template.md',
        '_draft-mix.md',
        '2025-01-01-published-mix.md',
      ] as unknown as fs.Dirent[])

      vi.resetModules()
      const { getMixSlugs } = await import('./mixes')
      const slugs = getMixSlugs()

      expect(slugs).toEqual(['2025-01-01-published-mix'])
    })

    it('returns empty array when no files exist', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([])

      vi.resetModules()
      const { getMixSlugs } = await import('./mixes')
      const slugs = getMixSlugs()

      expect(slugs).toEqual([])
    })
  })

  describe('getMix', () => {
    it('returns null when file does not exist', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(false)

      vi.resetModules()
      const { getMix } = await import('./mixes')
      const mix = getMix('non-existent-mix')

      expect(mix).toBeNull()
    })

    it('parses frontmatter and content correctly', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Vinyl Mix Vol. 1"
date: "2025-01-15"
description: "A vinyl-only DJ set"
artist: "DJ Test"
soundcloud_url: "https://soundcloud.com/djtest/vinyl-mix-1"
artist_url: "https://instagram.com/djtest"
---

This is a description of the mix with track listing.

1. Artist - Track Name
2. Another Artist - Another Track
`)

      vi.resetModules()
      const { getMix } = await import('./mixes')
      const mix = getMix('2025-01-15-vinyl-mix')

      expect(mix).not.toBeNull()
      expect(mix?.slug).toBe('2025-01-15-vinyl-mix')
      expect(mix?.frontmatter.title).toBe('Vinyl Mix Vol. 1')
      expect(mix?.frontmatter.date).toBe('2025-01-15')
      expect(mix?.frontmatter.artist).toBe('DJ Test')
      expect(mix?.frontmatter.soundcloud_url).toBe(
        'https://soundcloud.com/djtest/vinyl-mix-1'
      )
      expect(mix?.content).toContain('description of the mix')
    })

    it('converts soundcloud shortcodes to MDX', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Mix with Embed"
date: "2025-02-01"
---

Here's the mix:

{{< soundcloud url="https://soundcloud.com/artist/track" height="300" >}}
`)

      vi.resetModules()
      const { getMix } = await import('./mixes')
      const mix = getMix('2025-02-01-embed-mix')

      expect(mix?.content).toContain('<SoundCloud')
      expect(mix?.content).toContain('url="https://soundcloud.com/artist/track"')
      expect(mix?.content).toContain('height="300"')
      expect(mix?.content).not.toContain('{{<')
    })

    it('handles mix without soundcloud shortcodes', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Plain Mix"
date: "2025-03-01"
soundcloud_url: "https://soundcloud.com/test"
---

Just text content with the track listing.
`)

      vi.resetModules()
      const { getMix } = await import('./mixes')
      const mix = getMix('2025-03-01-plain-mix')

      expect(mix?.content).toBe('\nJust text content with the track listing.\n')
    })

    it('includes all frontmatter fields', async () => {
      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Full Mix"
date: "2025-04-01"
description: "Complete mix with all fields"
artist: "Artist Name"
soundcloud_url: "https://soundcloud.com/artist/mix"
artist_url: "https://artist-website.com"
track_url: "https://example.com/download"
---
Content`)

      vi.resetModules()
      const { getMix } = await import('./mixes')
      const mix = getMix('2025-04-01-full-mix')

      expect(mix?.frontmatter).toMatchObject({
        title: 'Full Mix',
        date: '2025-04-01',
        description: 'Complete mix with all fields',
        artist: 'Artist Name',
        soundcloud_url: 'https://soundcloud.com/artist/mix',
        artist_url: 'https://artist-website.com',
        track_url: 'https://example.com/download',
      })
    })
  })

  describe('getAllMixes', () => {
    it('returns mixes sorted by date (newest first)', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-01-old-mix.md',
        '2025-03-01-new-mix.md',
        '2025-02-01-middle-mix.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)

      vi.mocked(fs.readFileSync).mockImplementation(
        (filePath: fs.PathOrFileDescriptor) => {
          const path = filePath.toString()
          if (path.includes('2025-01-01')) {
            return `---
title: "Old Mix"
date: "2025-01-01"
artist: "DJ One"
---
Content`
          }
          if (path.includes('2025-02-01')) {
            return `---
title: "Middle Mix"
date: "2025-02-01"
artist: "DJ Two"
---
Content`
          }
          return `---
title: "New Mix"
date: "2025-03-01"
artist: "DJ Three"
---
Content`
        }
      )

      vi.resetModules()
      const { getAllMixes } = await import('./mixes')
      const mixes = getAllMixes()

      expect(mixes.length).toBe(3)
      expect(mixes[0].title).toBe('New Mix')
      expect(mixes[1].title).toBe('Middle Mix')
      expect(mixes[2].title).toBe('Old Mix')
    })

    it('includes metadata for each mix', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        '2025-01-15-test-mix.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Test Mix"
date: "2025-01-15"
description: "A test mix description"
artist: "Test DJ"
soundcloud_url: "https://soundcloud.com/test/mix"
artist_url: "https://testdj.com"
track_url: "https://download.com/mix"
---
Mix content`)

      vi.resetModules()
      const { getAllMixes } = await import('./mixes')
      const mixes = getAllMixes()

      expect(mixes[0]).toMatchObject({
        slug: '2025-01-15-test-mix',
        title: 'Test Mix',
        date: '2025-01-15',
        description: 'A test mix description',
        artist: 'Test DJ',
        soundcloud_url: 'https://soundcloud.com/test/mix',
        artist_url: 'https://testdj.com',
        track_url: 'https://download.com/mix',
      })
    })

    it('skips mixes that fail to parse', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'valid.md',
        'invalid.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockImplementation(
        (path: fs.PathLike) => !path.toString().includes('invalid')
      )
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Valid Mix"
date: "2025-01-01"
---
Content`)

      vi.resetModules()
      const { getAllMixes } = await import('./mixes')
      const mixes = getAllMixes()

      expect(mixes.length).toBe(1)
      expect(mixes[0].title).toBe('Valid Mix')
    })

    it('returns empty array when no mixes exist', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([])

      vi.resetModules()
      const { getAllMixes } = await import('./mixes')
      const mixes = getAllMixes()

      expect(mixes).toEqual([])
    })

    it('handles mixes with minimal frontmatter', async () => {
      vi.mocked(fs.readdirSync).mockReturnValue([
        'minimal.md',
      ] as unknown as fs.Dirent[])

      vi.mocked(fs.existsSync).mockReturnValue(true)
      vi.mocked(fs.readFileSync).mockReturnValue(`---
title: "Minimal Mix"
date: "2025-05-01"
---
Content`)

      vi.resetModules()
      const { getAllMixes } = await import('./mixes')
      const mixes = getAllMixes()

      expect(mixes[0].title).toBe('Minimal Mix')
      expect(mixes[0].date).toBe('2025-05-01')
      expect(mixes[0].artist).toBeUndefined()
      expect(mixes[0].soundcloud_url).toBeUndefined()
    })
  })
})
