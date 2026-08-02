import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { revalidatePath } from 'next/cache'
import * as Sentry from '@sentry/nextjs'
import {
  MAX_BODY_BYTES,
  MAX_ENTITIES_PER_REQUEST,
  isAuthorized,
  parseRevalidateBody,
  revalidateEntities,
} from './internal-revalidate'

vi.mock('next/cache', () => ({
  revalidatePath: vi.fn(),
}))

const mockRevalidatePath = vi.mocked(revalidatePath)
const mockCaptureMessage = vi.mocked(Sentry.captureMessage)

// Realistic shape: the backend refuses to boot with a secret under 32 chars.
const SECRET = 'a'.repeat(40)

beforeEach(() => {
  vi.resetAllMocks()
})

afterEach(() => {
  vi.unstubAllEnvs()
})

/** Every path revalidated during the current test. */
function revalidated(): string[] {
  return mockRevalidatePath.mock.calls.map(([path]) => path as string)
}

describe('isAuthorized', () => {
  it('accepts the configured secret', () => {
    vi.stubEnv('INTERNAL_API_SECRET', SECRET)
    expect(isAuthorized(SECRET)).toBe(true)
  })

  it('rejects a wrong secret of the same length', () => {
    vi.stubEnv('INTERNAL_API_SECRET', SECRET)
    expect(isAuthorized('b'.repeat(40))).toBe(false)
  })

  it('rejects a secret of a different length without throwing', () => {
    vi.stubEnv('INTERNAL_API_SECRET', SECRET)
    // timingSafeEqual throws on length mismatch; hashing both sides first is
    // what keeps this a plain false rather than a 500 (and a length oracle).
    expect(isAuthorized('short')).toBe(false)
    expect(isAuthorized('a'.repeat(400))).toBe(false)
  })

  it('rejects a missing or empty header', () => {
    vi.stubEnv('INTERNAL_API_SECRET', SECRET)
    expect(isAuthorized(null)).toBe(false)
    expect(isAuthorized(undefined)).toBe(false)
    expect(isAuthorized('')).toBe(false)
  })

  it('rejects everything when the secret is unconfigured, and reports it', () => {
    vi.stubEnv('INTERNAL_API_SECRET', '')
    expect(isAuthorized(SECRET)).toBe(false)
    expect(isAuthorized('')).toBe(false)
    expect(mockCaptureMessage).toHaveBeenCalledWith(
      expect.stringContaining('INTERNAL_API_SECRET unset'),
      expect.objectContaining({ level: 'warning' })
    )
  })
})

describe('parseRevalidateBody', () => {
  it('accepts a well-formed envelope', () => {
    const parsed = parseRevalidateBody(
      JSON.stringify({ entities: [{ type: 'show', slug: 'a-show' }] })
    )
    expect(parsed).toEqual({ entities: [{ type: 'show', slug: 'a-show' }] })
  })

  it('accepts an empty batch', () => {
    expect(parseRevalidateBody('{"entities":[]}')).toEqual({ entities: [] })
  })

  it('rejects non-JSON', () => {
    expect(parseRevalidateBody('not json')).toEqual({
      error: 'Invalid JSON body',
    })
  })

  it('rejects a JSON scalar or array envelope', () => {
    expect(parseRevalidateBody('"hi"')).toHaveProperty('error')
    expect(parseRevalidateBody('[]')).toHaveProperty('error')
    expect(parseRevalidateBody('null')).toHaveProperty('error')
  })

  it('rejects a missing or non-array entities field', () => {
    expect(parseRevalidateBody('{}')).toEqual({
      error: 'Expected an "entities" array',
    })
    expect(parseRevalidateBody('{"entities":"show"}')).toEqual({
      error: 'Expected an "entities" array',
    })
  })

  it('rejects a batch over the cap', () => {
    const entities = Array.from({ length: MAX_ENTITIES_PER_REQUEST + 1 }, () => ({
      type: 'show',
      slug: 'a-show',
    }))
    expect(parseRevalidateBody(JSON.stringify({ entities }))).toEqual({
      error: `Too many entities (max ${MAX_ENTITIES_PER_REQUEST} per request)`,
    })
  })

  it('accepts a batch exactly at the cap', () => {
    const entities = Array.from({ length: MAX_ENTITIES_PER_REQUEST }, () => ({
      type: 'show',
      slug: 'a-show',
    }))
    expect(parseRevalidateBody(JSON.stringify({ entities }))).not.toHaveProperty(
      'error'
    )
  })

  it('rejects an oversized body before parsing it', () => {
    const oversized = `{"entities":[],"pad":"${'x'.repeat(MAX_BODY_BYTES)}"}`
    expect(parseRevalidateBody(oversized)).toEqual({
      error: `Body exceeds ${MAX_BODY_BYTES} bytes`,
    })
  })
})

describe('revalidateEntities', () => {
  it('revalidates a show detail page and the surfaces its data feeds', () => {
    const result = revalidateEntities([{ type: 'show', slug: 'a-big-gig' }])

    expect(revalidated()).toEqual(
      expect.arrayContaining([
        '/shows/a-big-gig',
        '/shows',
        '/explore',
        '/artists',
        '/venues',
        '/scenes',
        '/scenes/[slug]',
        '/collections/[slug]',
      ])
    )
    expect(result).toEqual({
      revalidated: 1,
      skipped: 0,
      paths: revalidated().length,
    })
  })

  it('revalidates an artist page plus the rename cascade', () => {
    revalidateEntities([{ type: 'artist', slug: 'bright-eyes' }])

    expect(revalidated()).toEqual(
      expect.arrayContaining([
        '/artists/bright-eyes',
        '/artists',
        '/shows',
        '/shows/[slug]',
        '/releases/[slug]',
        '/collections/[slug]',
      ])
    )
  })

  it('revalidates the scene list for a venue (per-city venue counts)', () => {
    revalidateEntities([{ type: 'venue', slug: 'the-rebel-lounge' }])

    expect(revalidated()).toEqual(
      expect.arrayContaining(['/venues/the-rebel-lounge', '/venues', '/scenes'])
    )
  })

  it('accepts tags, which the proxy entity_type vocabulary excludes', () => {
    revalidateEntities([{ type: 'tag', slug: 'post-punk' }])
    expect(revalidated()).toContain('/tags/post-punk')
  })

  it('revalidates each shared path only once across a batch', () => {
    revalidateEntities([
      { type: 'show', slug: 'gig-one' },
      { type: 'show', slug: 'gig-two' },
      { type: 'show', slug: 'gig-three' },
    ])

    const paths = revalidated()
    expect(new Set(paths).size).toBe(paths.length)
    expect(paths).toEqual(
      expect.arrayContaining(['/shows/gig-one', '/shows/gig-two', '/shows/gig-three'])
    )
    expect(paths.filter((path) => path === '/shows')).toHaveLength(1)
  })

  it('does nothing for an empty batch', () => {
    expect(revalidateEntities([])).toEqual({
      revalidated: 0,
      skipped: 0,
      paths: 0,
    })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('skips unknown entity types', () => {
    const result = revalidateEntities([
      { type: 'user', slug: 'mtrifilo' },
      { type: 'constructor', slug: 'nope' },
      { type: '__proto__', slug: 'nope' },
    ])

    expect(result.revalidated).toBe(0)
    expect(result.skipped).toBe(3)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('skips slugs that are not backend slug shape', () => {
    const result = revalidateEntities([
      { type: 'show', slug: '../../../etc/passwd' },
      { type: 'show', slug: 'a/b' },
      { type: 'show', slug: '[slug]' },
      { type: 'show', slug: 'Trailing-Caps' },
      { type: 'show', slug: '-leading-hyphen' },
      { type: 'show', slug: 'trailing-hyphen-' },
      { type: 'show', slug: 'double--hyphen' },
      { type: 'show', slug: '' },
      { type: 'show', slug: `${'a'.repeat(201)}` },
      { type: 'show', slug: 'ok\nnewline' },
      { type: 'show', slug: 42 },
      { type: 'show' },
      null,
      'a-show',
    ])

    expect(result.revalidated).toBe(0)
    expect(result.skipped).toBe(14)
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('revalidates the good entries in a batch that also has bad ones', () => {
    const result = revalidateEntities([
      { type: 'show', slug: 'a-real-show' },
      { type: 'nonsense', slug: 'whatever' },
    ])

    expect(result).toMatchObject({ revalidated: 1, skipped: 1 })
    expect(revalidated()).toContain('/shows/a-real-show')
    expect(mockCaptureMessage).toHaveBeenCalledWith(
      expect.stringContaining('skipped unusable entities'),
      expect.objectContaining({ level: 'warning' })
    )
  })

  it('passes dynamic route patterns through as patterns, not literal paths', () => {
    revalidateEntities([{ type: 'show', slug: 'a-big-gig' }])

    // safeRevalidatePath must call revalidatePath(pattern, 'page') for
    // bracketed routes and revalidatePath(path) for concrete ones.
    expect(mockRevalidatePath).toHaveBeenCalledWith('/scenes/[slug]', 'page')
    expect(mockRevalidatePath).toHaveBeenCalledWith('/collections/[slug]', 'page')
    expect(mockRevalidatePath).toHaveBeenCalledWith('/shows/a-big-gig')
  })
})
