import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { QueryKey } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queryClient'
import { clientWithAppDefaults } from '@/test/inflightOrdering'
import {
  invalidatedKeys,
  mutationCase,
  runMutation,
} from '@/test/mutationRefetchKeys'
import {
  useAddCollectionItem,
  useAddCollectionTag,
  useBulkAddCollectionItems,
  useCloneCollection,
  useCreateCollection,
  useDeleteCollection,
  useLikeCollection,
  useRemoveCollectionItem,
  useRemoveCollectionTag,
  useReorderCollectionItems,
  useSetFeatured,
  useSubscribeCollection,
  useUnlikeCollection,
  useUnsubscribeCollection,
  useUpdateCollection,
  useUpdateCollectionItem,
} from './index'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const keys = queryKeys.collections
const list = keys.list(undefined)
const my = keys.my
const detail = keys.detail('mine')
const containing = keys.containing('artist', 5)
const entity = keys.entity('artist', 5)
const userPublic = keys.userPublic('me')
const seeded: QueryKey[] = [list, my, detail, containing, entity, userPublic]

function seededClient() {
  const client = clientWithAppDefaults()
  for (const key of seeded) client.setQueryData(key, { seeded: true })
  return client
}

describe('what each collection mutation refetches', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    mockApiRequest.mockResolvedValue({})
  })

  it.each([
    ['useSetFeatured', mutationCase(useSetFeatured, { slug: 'mine', featured: true })],
    [
      'useCreateCollection',
      mutationCase(useCreateCollection, {
        title: 'New',
        is_public: true,
        collaborative: false,
      }),
    ],
    [
      'useUpdateCollection',
      mutationCase(useUpdateCollection, { slug: 'mine', title: 'Renamed' }),
    ],
    ['useDeleteCollection', mutationCase(useDeleteCollection, { slug: 'mine' })],
    ['useLikeCollection', mutationCase(useLikeCollection, { slug: 'mine' })],
    ['useUnlikeCollection', mutationCase(useUnlikeCollection, { slug: 'mine' })],
    [
      'useAddCollectionTag',
      mutationCase(useAddCollectionTag, { slug: 'mine', tag_id: 3 }),
    ],
    [
      'useRemoveCollectionTag',
      mutationCase(useRemoveCollectionTag, { slug: 'mine', tagId: 3 }),
    ],
  ])('%s refetches the whole family', async (_name, run) => {
    const client = seededClient()
    await run(client)
    expect(invalidatedKeys(client, seeded)).toEqual(seeded)
  })

  it('useCloneCollection refetches the family but not the clone it pre-warms', async () => {
    const clone = { id: 9, slug: 'mine-copy', title: 'Mine (copy)' }
    mockApiRequest.mockResolvedValue(clone)
    const client = seededClient()
    await runMutation(client, useCloneCollection, { slug: 'mine' })

    expect(invalidatedKeys(client, seeded)).toEqual(seeded)
    expect(client.getQueryData(keys.detail('mine-copy'))).toEqual(clone)
    expect(client.getQueryState(keys.detail('mine-copy'))?.isInvalidated).toBe(false)
  })

  it('useAddCollectionItem refetches the detail, the entity lookups and the viewer list', async () => {
    const client = seededClient()
    await runMutation(client, useAddCollectionItem, {
      slug: 'mine',
      entityType: 'artist',
      entityId: 5,
    })
    expect(invalidatedKeys(client, seeded)).toEqual([my, detail, containing, entity])
  })

  it('useBulkAddCollectionItems refetches the detail and the lookups of rows that committed', async () => {
    mockApiRequest.mockResolvedValue({
      added: [{ id: 1, entity_type: 'artist', entity_id: 5 }],
      errors: [{ row_index: 1, error_code: 'DUPLICATE', message: 'dupe' }],
    })
    const client = seededClient()
    await runMutation(client, useBulkAddCollectionItems, {
      slug: 'mine',
      items: [
        { entity_type: 'artist', entity_id: 5 },
        { entity_type: 'artist', entity_id: 6 },
      ],
    })
    expect(invalidatedKeys(client, seeded)).toEqual([detail, containing, entity])
  })

  it('useRemoveCollectionItem refetches the detail and the viewer list', async () => {
    const client = seededClient()
    await runMutation(client, useRemoveCollectionItem, { slug: 'mine', itemId: 1 })
    expect(invalidatedKeys(client, seeded)).toEqual([my, detail])
  })

  it.each([
    [
      'useReorderCollectionItems',
      mutationCase(useReorderCollectionItems, {
        slug: 'mine',
        items: [{ item_id: 1, position: 0 }],
      }),
    ],
    [
      'useUpdateCollectionItem',
      mutationCase(useUpdateCollectionItem, { slug: 'mine', itemId: 1, notes: 'n' }),
    ],
    ['useSubscribeCollection', mutationCase(useSubscribeCollection, { slug: 'mine' })],
    [
      'useUnsubscribeCollection',
      mutationCase(useUnsubscribeCollection, { slug: 'mine' }),
    ],
  ])('%s refetches only the detail', async (_name, run) => {
    const client = seededClient()
    await run(client)
    expect(invalidatedKeys(client, seeded)).toEqual([detail])
  })
})
