import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { revalidatePath } from 'next/cache'
import * as Sentry from '@sentry/nextjs'
import {
  isMutationMethod,
  revalidateAfterProxyMutation,
  type ProxyMutationArgs,
} from './proxy-revalidation'

vi.mock('next/cache', () => ({
  revalidatePath: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

const mockRevalidatePath = vi.mocked(revalidatePath)
const mockCaptureMessage = vi.mocked(Sentry.captureMessage)
const mockCaptureException = vi.mocked(Sentry.captureException)

// BACKEND_URL is unset in the vitest env, so lookups fall back to this.
const BACKEND = 'http://localhost:8080'

let fetchSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  // resetAllMocks (not clearAllMocks) so mockImplementation overrides from
  // throwing-path tests don't leak into later tests.
  vi.resetAllMocks()
  fetchSpy = vi.spyOn(globalThis, 'fetch')
})

afterEach(() => {
  fetchSpy.mockRestore()
})

/** Run the engine with sane defaults for the args not under test. */
function run(
  args: Partial<ProxyMutationArgs> & Pick<ProxyMutationArgs, 'method' | 'path'>
) {
  return revalidateAfterProxyMutation({
    responseText: null,
    requestText: null,
    cookieHeader: undefined,
    ...args,
  })
}

/** Every path revalidated during the current test, in call order. */
function revalidated(): string[] {
  return mockRevalidatePath.mock.calls.map(([path]) => path as string)
}

/** Mock the next lookup GET to return an entity with the given slug. */
function mockLookupResponse(slug: string) {
  fetchSpy.mockResolvedValueOnce(
    new Response(JSON.stringify({ id: 1, slug }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  )
}

describe('isMutationMethod', () => {
  it.each(['POST', 'PUT', 'PATCH', 'DELETE'])('%s is a mutation', (method) => {
    expect(isMutationMethod(method)).toBe(true)
  })

  it.each(['GET', 'HEAD', 'OPTIONS'])('%s is not a mutation', (method) => {
    expect(isMutationMethod(method)).toBe(false)
  })
})

describe('no-op cases', () => {
  it('does nothing for GET requests', async () => {
    await run({ method: 'GET', path: '/shows' })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('does nothing for mutation paths with no matching rule', async () => {
    await run({ method: 'POST', path: '/auth/login' })
    await run({ method: 'POST', path: '/comments/5/vote' })
    await run({ method: 'POST', path: '/saved-shows/12' })
    await run({ method: 'POST', path: '/requests/3/vote' })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('does nothing when the path matches a rule pattern but not its method', async () => {
    // /shows only has POST (create) — PATCH matches no rule.
    await run({ method: 'PATCH', path: '/shows' })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })
})

describe('show rules', () => {
  const showBody = JSON.stringify({
    id: 7,
    slug: 'bright-eyes-at-the-rebel-lounge-2026-07-01',
    artists: [{ id: 1, slug: 'bright-eyes' }, { id: 2, slug: 'cursive' }],
    venues: [{ id: 3, slug: 'the-rebel-lounge' }],
  })

  const expectedShowPages = [
    '/shows/bright-eyes-at-the-rebel-lounge-2026-07-01',
    '/shows',
    '/explore',
    '/artists',
    '/venues',
    '/scenes/[slug]',
    '/artists/bright-eyes',
    '/artists/cursive',
  ]

  it('show create revalidates the show, list surfaces, scenes, and billed artists', async () => {
    await run({ method: 'POST', path: '/shows', responseText: showBody })
    expect(revalidated()).toEqual(expectedShowPages)
  })

  it('show update revalidates the same set plus the collection cascade', async () => {
    await run({ method: 'PUT', path: '/shows/7', responseText: showBody })
    // Title edits stale collection pages embedding the show's name.
    expect(revalidated()).toEqual([...expectedShowPages, '/collections/[slug]'])
  })

  it('show status ops revalidate the same set', async () => {
    for (const op of [
      'publish',
      'unpublish',
      'make-private',
      'sold-out',
      'cancelled',
    ]) {
      mockRevalidatePath.mockClear()
      await run({
        method: 'POST',
        path: `/shows/7/${op}`,
        responseText: showBody,
      })
      expect(revalidated()).toEqual(expectedShowPages)
    }
  })

  it('admin show approve/reject revalidates the same set', async () => {
    await run({
      method: 'POST',
      path: '/admin/shows/7/approve',
      responseText: showBody,
    })
    expect(revalidated()).toEqual(expectedShowPages)
  })

  it('batch approve/reject blasts the show route (response has counts, not slugs)', async () => {
    const expectedBatchPages = [
      '/shows/[slug]',
      '/shows',
      '/explore',
      '/artists',
      '/venues',
      '/scenes/[slug]',
    ]
    await run({
      method: 'POST',
      path: '/admin/shows/batch-approve',
      responseText: JSON.stringify({ approved: 50, errors: [] }),
    })
    expect(revalidated()).toEqual(expectedBatchPages)

    mockRevalidatePath.mockClear()
    await run({
      method: 'POST',
      path: '/admin/shows/batch-reject',
      responseText: JSON.stringify({ rejected: 3, errors: [] }),
    })
    expect(revalidated()).toEqual(expectedBatchPages)
  })

  it('show delete revalidates list surfaces, scenes, and the collection cascade', async () => {
    await run({ method: 'DELETE', path: '/shows/7' })
    expect(revalidated()).toEqual([
      '/shows',
      '/explore',
      '/artists',
      '/venues',
      '/scenes/[slug]',
      '/collections/[slug]',
    ])
  })

  it('degrades to list surfaces when the response body is not JSON', async () => {
    await run({ method: 'POST', path: '/shows', responseText: 'created' })
    expect(revalidated()).toEqual([
      '/shows',
      '/explore',
      '/artists',
      '/venues',
      '/scenes/[slug]',
    ])
  })
})

describe('suggest-edit rules', () => {
  function suggestEditBody(applied: boolean, entitySlug?: string) {
    return JSON.stringify({
      applied,
      message: 'ok',
      pending_edit: {
        id: 1,
        entity_type: 'artist',
        entity_id: 10,
        entity_slug: entitySlug,
      },
    })
  }

  it('revalidates the entity + list page + rename cascade when the edit was auto-applied', async () => {
    await run({
      method: 'PUT',
      path: '/artists/10/suggest-edit',
      responseText: suggestEditBody(true, 'bright-eyes'),
    })
    expect(revalidated()).toEqual([
      '/artists/bright-eyes',
      '/artists',
      '/shows/[slug]',
      '/releases/[slug]',
      '/collections/[slug]',
    ])
  })

  it('does nothing when the edit went to the pending queue (applied: false)', async () => {
    await run({
      method: 'PUT',
      path: '/artists/10/suggest-edit',
      responseText: suggestEditBody(false, 'bright-eyes'),
    })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('revalidates the list page + cascade when the slug is missing', async () => {
    await run({
      method: 'PUT',
      path: '/artists/10/suggest-edit',
      responseText: suggestEditBody(true, undefined),
    })
    expect(revalidated()).toEqual([
      '/artists',
      '/shows/[slug]',
      '/releases/[slug]',
      '/collections/[slug]',
    ])
  })

  it('uses the entity type from the path (venue)', async () => {
    await run({
      method: 'PUT',
      path: '/venues/4/suggest-edit',
      responseText: suggestEditBody(true, 'the-rebel-lounge'),
    })
    expect(revalidated()).toEqual([
      '/venues/the-rebel-lounge',
      '/venues',
      '/shows/[slug]',
      '/collections/[slug]',
    ])
  })

  it('skips the list page for entity types without an ISR list route (release)', async () => {
    await run({
      method: 'PUT',
      path: '/releases/9/suggest-edit',
      responseText: suggestEditBody(true, 'fevers-and-mirrors'),
    })
    expect(revalidated()).toEqual([
      '/releases/fevers-and-mirrors',
      '/collections/[slug]',
    ])
  })
})

describe('pending-edit moderation rules', () => {
  it('approve revalidates the affected entity + list page + rename cascade', async () => {
    await run({
      method: 'POST',
      path: '/admin/pending-edits/55/approve',
      responseText: JSON.stringify({
        id: 55,
        entity_type: 'artist',
        entity_slug: 'bright-eyes',
      }),
    })
    expect(revalidated()).toEqual([
      '/artists/bright-eyes',
      '/artists',
      '/shows/[slug]',
      '/releases/[slug]',
      '/collections/[slug]',
    ])
  })

  it('approve maps singular entity types without ISR list pages (label)', async () => {
    await run({
      method: 'POST',
      path: '/admin/pending-edits/55/approve',
      responseText: JSON.stringify({
        id: 55,
        entity_type: 'label',
        entity_slug: 'saddle-creek',
      }),
    })
    expect(revalidated()).toEqual([
      '/labels/saddle-creek',
      '/releases/[slug]',
      '/collections/[slug]',
    ])
  })

  it('approve does nothing for unknown entity types', async () => {
    await run({
      method: 'POST',
      path: '/admin/pending-edits/55/approve',
      responseText: JSON.stringify({
        id: 55,
        entity_type: 'mixtape',
        entity_slug: 'whatever',
      }),
    })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('reject has no rule (entity unchanged)', async () => {
    await run({
      method: 'POST',
      path: '/admin/pending-edits/55/reject',
      responseText: JSON.stringify({
        id: 55,
        entity_type: 'artist',
        entity_slug: 'bright-eyes',
      }),
    })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })
})

describe('artist rules', () => {
  it('admin create revalidates the artist + list', async () => {
    await run({
      method: 'POST',
      path: '/admin/artists',
      responseText: JSON.stringify({ id: 1, slug: 'new-artist' }),
    })
    expect(revalidated()).toEqual(['/artists/new-artist', '/artists'])
  })

  // Renames/merges/deletes change the artist name embedded in show, release,
  // and collection ISR payloads → the artist cascade.
  const artistCascade = ['/shows/[slug]', '/releases/[slug]', '/collections/[slug]']

  it('admin update revalidates the artist + list + rename cascade', async () => {
    await run({
      method: 'PATCH',
      path: '/admin/artists/1',
      responseText: JSON.stringify({ id: 1, slug: 'renamed-artist' }),
    })
    expect(revalidated()).toEqual([
      '/artists/renamed-artist',
      '/artists',
      ...artistCascade,
    ])
  })

  it('delete revalidates the list + rename cascade', async () => {
    await run({ method: 'DELETE', path: '/artists/1' })
    expect(revalidated()).toEqual(['/artists', ...artistCascade])
  })

  it('merge resolves the canonical artist slug via backend lookup', async () => {
    mockLookupResponse('bright-eyes')
    await run({
      method: 'POST',
      path: '/admin/artists/merge',
      responseText: JSON.stringify({
        canonical_artist_id: 42,
        merged_artist_id: 99,
      }),
    })
    expect(fetchSpy).toHaveBeenCalledWith(
      `${BACKEND}/artists/42`,
      expect.objectContaining({ cache: 'no-store' })
    )
    expect(revalidated()).toEqual([
      '/artists/bright-eyes',
      '/artists',
      ...artistCascade,
    ])
  })

  it('merge falls back to the list page + cascade when the lookup fails', async () => {
    fetchSpy.mockResolvedValueOnce(new Response('gone', { status: 404 }))
    await run({
      method: 'POST',
      path: '/admin/artists/merge',
      responseText: JSON.stringify({ canonical_artist_id: 42 }),
    })
    expect(revalidated()).toEqual(['/artists', ...artistCascade])
    // Lookup failures are observable so the staleness gap is visible.
    expect(mockCaptureMessage).toHaveBeenCalledWith(
      'isr-revalidation: slug lookup failed',
      expect.objectContaining({ level: 'warning' })
    )
  })
})

describe('venue rules', () => {
  it('admin create revalidates the venue + list', async () => {
    await run({
      method: 'POST',
      path: '/admin/venues',
      responseText: JSON.stringify({ id: 3, slug: 'new-venue' }),
    })
    expect(revalidated()).toEqual(['/venues/new-venue', '/venues'])
  })

  // Venue renames/deletes change the venue name embedded in show and
  // collection ISR payloads (releases don't credit venues).
  const venueCascade = ['/shows/[slug]', '/collections/[slug]']

  it('update revalidates the venue + list + rename cascade', async () => {
    await run({
      method: 'PUT',
      path: '/venues/3',
      responseText: JSON.stringify({ id: 3, slug: 'the-rebel-lounge' }),
    })
    expect(revalidated()).toEqual([
      '/venues/the-rebel-lounge',
      '/venues',
      ...venueCascade,
    ])
  })

  it('delete revalidates the list + rename cascade', async () => {
    await run({ method: 'DELETE', path: '/venues/3' })
    expect(revalidated()).toEqual(['/venues', ...venueCascade])
  })

  it('verify revalidates the venue + list with NO cascade (name unchanged)', async () => {
    await run({
      method: 'POST',
      path: '/admin/venues/3/verify',
      responseText: JSON.stringify({ id: 3, slug: 'the-rebel-lounge' }),
    })
    expect(revalidated()).toEqual(['/venues/the-rebel-lounge', '/venues'])
  })
})

describe('release rules', () => {
  it('create revalidates the release + credited artist pages', async () => {
    await run({
      method: 'POST',
      path: '/releases',
      responseText: JSON.stringify({
        id: 9,
        slug: 'fevers-and-mirrors',
        artists: [{ id: 1, slug: 'bright-eyes' }],
      }),
    })
    expect(revalidated()).toEqual([
      '/releases/fevers-and-mirrors',
      '/artists/bright-eyes',
    ])
  })

  it('update revalidates the release + credited artist pages + collection cascade', async () => {
    await run({
      method: 'PUT',
      path: '/releases/9',
      responseText: JSON.stringify({
        id: 9,
        slug: 'fevers-and-mirrors',
        artists: [{ id: 1, slug: 'bright-eyes' }],
      }),
    })
    expect(revalidated()).toEqual([
      '/releases/fevers-and-mirrors',
      '/artists/bright-eyes',
      '/collections/[slug]',
    ])
  })

  it('delete revalidates the collection cascade (no ISR list page for releases)', async () => {
    await run({ method: 'DELETE', path: '/releases/9' })
    expect(revalidated()).toEqual(['/collections/[slug]'])
  })

  it('link add resolves the release slug from the path ID', async () => {
    mockLookupResponse('fevers-and-mirrors')
    await run({ method: 'POST', path: '/releases/9/links' })
    expect(fetchSpy).toHaveBeenCalledWith(
      `${BACKEND}/releases/9`,
      expect.anything()
    )
    expect(revalidated()).toEqual(['/releases/fevers-and-mirrors'])
  })

  it('link remove resolves the release slug from the path ID', async () => {
    mockLookupResponse('fevers-and-mirrors')
    await run({ method: 'DELETE', path: '/releases/9/links/2' })
    expect(revalidated()).toEqual(['/releases/fevers-and-mirrors'])
  })
})

describe('label rules', () => {
  it('create revalidates the label page', async () => {
    await run({
      method: 'POST',
      path: '/labels',
      responseText: JSON.stringify({ id: 5, slug: 'saddle-creek' }),
    })
    expect(revalidated()).toEqual(['/labels/saddle-creek'])
  })

  it('update revalidates the label page + rename cascade', async () => {
    await run({
      method: 'PUT',
      path: '/labels/5',
      responseText: JSON.stringify({ id: 5, slug: 'saddle-creek' }),
    })
    // Release pages credit labels by name; collections may list the label.
    expect(revalidated()).toEqual([
      '/labels/saddle-creek',
      '/releases/[slug]',
      '/collections/[slug]',
    ])
  })

  it('delete revalidates the rename cascade', async () => {
    await run({ method: 'DELETE', path: '/labels/5' })
    expect(revalidated()).toEqual(['/releases/[slug]', '/collections/[slug]'])
  })

  it('roster ops resolve the label slug from the path ID', async () => {
    for (const subResource of ['artists', 'releases']) {
      mockRevalidatePath.mockClear()
      mockLookupResponse('saddle-creek')
      await run({ method: 'POST', path: `/admin/labels/5/${subResource}` })
      expect(revalidated()).toEqual(['/labels/saddle-creek'])
    }
    expect(fetchSpy).toHaveBeenCalledWith(
      `${BACKEND}/labels/5`,
      expect.anything()
    )
  })
})

describe('festival rules', () => {
  it('create revalidates the festival page', async () => {
    await run({
      method: 'POST',
      path: '/festivals',
      responseText: JSON.stringify({ id: 6, slug: 'riot-fest-2026' }),
    })
    expect(revalidated()).toEqual(['/festivals/riot-fest-2026'])
  })

  it('update revalidates the festival page + collection cascade', async () => {
    await run({
      method: 'PUT',
      path: '/festivals/6',
      responseText: JSON.stringify({ id: 6, slug: 'riot-fest-2026' }),
    })
    expect(revalidated()).toEqual([
      '/festivals/riot-fest-2026',
      '/collections/[slug]',
    ])
  })

  it('delete revalidates the collection cascade', async () => {
    await run({ method: 'DELETE', path: '/festivals/6' })
    expect(revalidated()).toEqual(['/collections/[slug]'])
  })

  it('lineup ops resolve the festival slug from the path ID', async () => {
    const lineupOps: Array<[string, string]> = [
      ['POST', '/festivals/6/artists'],
      ['PUT', '/festivals/6/artists/12'],
      ['DELETE', '/festivals/6/artists/12'],
      ['POST', '/festivals/6/venues'],
      ['DELETE', '/festivals/6/venues/3'],
    ]
    for (const [method, path] of lineupOps) {
      mockRevalidatePath.mockClear()
      mockLookupResponse('riot-fest-2026')
      await run({ method, path })
      expect(revalidated()).toEqual(['/festivals/riot-fest-2026'])
    }
  })
})

describe('collection rules', () => {
  it('create revalidates the new collection page', async () => {
    await run({
      method: 'POST',
      path: '/collections',
      responseText: JSON.stringify({ id: 1, slug: 'desert-punk-essentials' }),
    })
    expect(revalidated()).toEqual(['/collections/desert-punk-essentials'])
  })

  it('clone revalidates the newly created clone', async () => {
    await run({
      method: 'POST',
      path: '/collections/desert-punk-essentials/clone',
      responseText: JSON.stringify({ id: 2, slug: 'desert-punk-essentials-2' }),
    })
    expect(revalidated()).toEqual(['/collections/desert-punk-essentials-2'])
  })

  it('update revalidates both the path slug and the response slug (rename-safe)', async () => {
    await run({
      method: 'PUT',
      path: '/collections/old-name',
      responseText: JSON.stringify({ id: 1, slug: 'new-name' }),
    })
    expect(revalidated()).toEqual([
      '/collections/old-name',
      '/collections/new-name',
    ])
  })

  it('update dedupes when the slug did not change', async () => {
    await run({
      method: 'PUT',
      path: '/collections/same-name',
      responseText: JSON.stringify({ id: 1, slug: 'same-name' }),
    })
    expect(revalidated()).toEqual(['/collections/same-name'])
  })

  it('delete revalidates the collection page', async () => {
    await run({ method: 'DELETE', path: '/collections/desert-punk-essentials' })
    expect(revalidated()).toEqual(['/collections/desert-punk-essentials'])
  })

  it('feature revalidates the collection + /explore', async () => {
    await run({ method: 'PUT', path: '/collections/desert-punk-essentials/feature' })
    expect(revalidated()).toEqual([
      '/collections/desert-punk-essentials',
      '/explore',
    ])
  })

  it('engagement ops (items/like/subscribe/tags) revalidate the collection page', async () => {
    const engagementOps: Array<[string, string]> = [
      ['POST', '/collections/my-list/items'],
      ['POST', '/collections/my-list/items/bulk'],
      ['PATCH', '/collections/my-list/items/42'],
      ['DELETE', '/collections/my-list/items/42'],
      ['PUT', '/collections/my-list/items/reorder'],
      ['POST', '/collections/my-list/like'],
      ['DELETE', '/collections/my-list/like'],
      ['POST', '/collections/my-list/subscribe'],
      ['DELETE', '/collections/my-list/subscribe'],
      ['POST', '/collections/my-list/tags'],
      ['DELETE', '/collections/my-list/tags/3'],
    ]
    for (const [method, path] of engagementOps) {
      mockRevalidatePath.mockClear()
      await run({ method, path })
      expect(revalidated()).toEqual(['/collections/my-list'])
    }
  })

  it('resolve-items matches no rule (read-style POST)', async () => {
    await run({ method: 'POST', path: '/collections/resolve-items' })
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })
})

describe('tag rules', () => {
  it('create revalidates the new tag page', async () => {
    await run({
      method: 'POST',
      path: '/tags',
      responseText: JSON.stringify({ id: 3, slug: 'post-punk' }),
    })
    expect(revalidated()).toEqual(['/tags/post-punk'])
  })

  it('update revalidates the tag page', async () => {
    await run({
      method: 'PUT',
      path: '/tags/3',
      responseText: JSON.stringify({ id: 3, slug: 'post-punk' }),
    })
    expect(revalidated()).toEqual(['/tags/post-punk'])
  })

  it('entity tag add resolves the tag slug from the request body tag_id', async () => {
    mockLookupResponse('post-punk')
    await run({
      method: 'POST',
      path: '/entities/artist/10/tags',
      requestText: JSON.stringify({ tag_id: 3 }),
    })
    expect(fetchSpy).toHaveBeenCalledWith(`${BACKEND}/tags/3`, expect.anything())
    expect(revalidated()).toEqual(['/tags/post-punk'])
  })

  it('entity tag add by tag_name does nothing (brand-new tag has no cached page)', async () => {
    await run({
      method: 'POST',
      path: '/entities/artist/10/tags',
      requestText: JSON.stringify({ tag_name: 'desert rock', category: 'genre' }),
    })
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('entity tag remove resolves the tag slug from the path tag_id', async () => {
    mockLookupResponse('post-punk')
    await run({ method: 'DELETE', path: '/entities/venue/4/tags/3' })
    expect(fetchSpy).toHaveBeenCalledWith(`${BACKEND}/tags/3`, expect.anything())
    expect(revalidated()).toEqual(['/tags/post-punk'])
  })
})

describe('collection tagging via the generic entities path (PSY-940)', () => {
  /** Route lookup GETs to per-URL responses (tag 3 + collection 9). */
  function mockLookups() {
    fetchSpy.mockImplementation(((url: string | URL | Request) => {
      const target = String(url)
      if (target === `${BACKEND}/tags/3`) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: 3, slug: 'post-punk' }), {
            status: 200,
          })
        )
      }
      if (target === `${BACKEND}/collections/9`) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: 9, slug: 'desert-punk-essentials' }), {
            status: 200,
          })
        )
      }
      return Promise.resolve(new Response('not found', { status: 404 }))
    }) as typeof fetch)
  }

  it('tag add on a collection revalidates BOTH the tag page and the collection page', async () => {
    mockLookups()
    await run({
      method: 'POST',
      path: '/entities/collection/9/tags',
      requestText: JSON.stringify({ tag_id: 3 }),
    })
    // Collection ISR payloads embed tags[], unlike every other entity type.
    expect(revalidated()).toEqual([
      '/tags/post-punk',
      '/collections/desert-punk-essentials',
    ])
  })

  it('tag add by tag_name on a collection still revalidates the collection page', async () => {
    mockLookups()
    await run({
      method: 'POST',
      path: '/entities/collection/9/tags',
      requestText: JSON.stringify({ tag_name: 'desert rock', category: 'genre' }),
    })
    expect(revalidated()).toEqual(['/collections/desert-punk-essentials'])
  })

  it('tag remove on a collection revalidates BOTH pages', async () => {
    mockLookups()
    await run({ method: 'DELETE', path: '/entities/collection/9/tags/3' })
    expect(revalidated()).toEqual([
      '/tags/post-punk',
      '/collections/desert-punk-essentials',
    ])
  })

  it('falls back to the tag page only when the collection lookup fails', async () => {
    fetchSpy.mockImplementation(((url: string | URL | Request) => {
      const target = String(url)
      if (target === `${BACKEND}/tags/3`) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: 3, slug: 'post-punk' }), {
            status: 200,
          })
        )
      }
      return Promise.resolve(new Response('error', { status: 500 }))
    }) as typeof fetch)

    await run({ method: 'DELETE', path: '/entities/collection/9/tags/3' })
    expect(revalidated()).toEqual(['/tags/post-punk'])
    // The skipped collection page is observable via the lookup-failure warning.
    expect(mockCaptureMessage).toHaveBeenCalledWith(
      'isr-revalidation: slug lookup failed',
      expect.objectContaining({ level: 'warning' })
    )
  })

  it('non-collection entity tagging never looks up the entity', async () => {
    mockLookupResponse('post-punk')
    await run({ method: 'DELETE', path: '/entities/artist/10/tags/3' })
    // Exactly one lookup (the tag) — never a second for the artist.
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    expect(fetchSpy).toHaveBeenCalledWith(`${BACKEND}/tags/3`, expect.anything())
    expect(revalidated()).toEqual(['/tags/post-punk'])
  })
})

describe('explore curation rules', () => {
  it('featured slot set/delete revalidates /explore', async () => {
    await run({ method: 'POST', path: '/admin/featured-slots' })
    expect(revalidated()).toEqual(['/explore'])

    mockRevalidatePath.mockClear()
    await run({ method: 'DELETE', path: '/admin/featured-slots/collection' })
    expect(revalidated()).toEqual(['/explore'])
  })
})

describe('cascade invalidation (PSY-941)', () => {
  it('revalidates route patterns with type page and concrete paths without', async () => {
    await run({
      method: 'PATCH',
      path: '/admin/artists/1',
      responseText: JSON.stringify({ id: 1, slug: 'renamed-artist' }),
    })

    // Concrete paths: single-argument revalidatePath calls.
    expect(mockRevalidatePath).toHaveBeenCalledWith('/artists/renamed-artist')
    expect(mockRevalidatePath).toHaveBeenCalledWith('/artists')
    // Route patterns: type 'page' so Next invalidates every page under the route.
    expect(mockRevalidatePath).toHaveBeenCalledWith('/shows/[slug]', 'page')
    expect(mockRevalidatePath).toHaveBeenCalledWith('/releases/[slug]', 'page')
    expect(mockRevalidatePath).toHaveBeenCalledWith('/collections/[slug]', 'page')
  })

  it('does not cascade on creates (nothing embeds a brand-new entity)', async () => {
    const creates: Array<[string, string, string]> = [
      ['POST', '/admin/artists', JSON.stringify({ id: 1, slug: 'new-artist' })],
      ['POST', '/admin/venues', JSON.stringify({ id: 2, slug: 'new-venue' })],
      ['POST', '/labels', JSON.stringify({ id: 3, slug: 'new-label' })],
      ['POST', '/festivals', JSON.stringify({ id: 4, slug: 'new-fest' })],
    ]
    for (const [method, path, responseText] of creates) {
      mockRevalidatePath.mockClear()
      await run({ method, path, responseText })
      expect(revalidated()).not.toContain('/shows/[slug]')
      expect(revalidated()).not.toContain('/releases/[slug]')
      expect(revalidated()).not.toContain('/collections/[slug]')
    }
  })

  it('collection mutations do not cascade (collections are not embedded elsewhere)', async () => {
    await run({
      method: 'PUT',
      path: '/collections/my-list',
      responseText: JSON.stringify({ id: 1, slug: 'my-list' }),
    })
    expect(revalidated()).toEqual(['/collections/my-list'])
  })
})

describe('resilience', () => {
  it('never throws when revalidatePath throws — reports each failure to Sentry', async () => {
    mockRevalidatePath.mockImplementation(() => {
      throw new Error('static generation store missing')
    })

    await expect(
      run({ method: 'DELETE', path: '/shows/7' })
    ).resolves.toBeUndefined()
    // show-delete touches 4 list pages + scenes + the collection cascade;
    // each failure is captured separately.
    expect(mockCaptureException).toHaveBeenCalledTimes(6)
  })

  it('never throws when the lookup fetch rejects — skips the page and reports', async () => {
    fetchSpy.mockRejectedValueOnce(new Error('ECONNREFUSED'))

    await expect(
      run({ method: 'DELETE', path: '/entities/artist/10/tags/3' })
    ).resolves.toBeUndefined()
    expect(mockRevalidatePath).not.toHaveBeenCalled()
    expect(mockCaptureException).toHaveBeenCalledTimes(1)
  })

  it('never throws on malformed request body JSON', async () => {
    await expect(
      run({
        method: 'POST',
        path: '/entities/artist/10/tags',
        requestText: 'not json',
      })
    ).resolves.toBeUndefined()
    expect(mockRevalidatePath).not.toHaveBeenCalled()
  })

  it('forwards the auth cookie on lookup GETs', async () => {
    mockLookupResponse('post-punk')
    await run({
      method: 'DELETE',
      path: '/entities/artist/10/tags/3',
      cookieHeader: 'auth_token=abc123',
    })
    const init = fetchSpy.mock.calls[0][1] as RequestInit
    expect((init.headers as Record<string, string>)['Cookie']).toBe(
      'auth_token=abc123'
    )
  })

  it('omits the cookie header on lookup GETs when not authenticated', async () => {
    mockLookupResponse('post-punk')
    await run({ method: 'DELETE', path: '/entities/artist/10/tags/3' })
    const init = fetchSpy.mock.calls[0][1] as RequestInit
    expect((init.headers as Record<string, string>)['Cookie']).toBeUndefined()
  })
})
