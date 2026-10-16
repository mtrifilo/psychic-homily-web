import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { QueryKey } from '@tanstack/react-query'
import { clientWithAppDefaults } from '@/test/inflightOrdering'
import { invalidatedKeys, mutationCase } from '@/test/mutationRefetchKeys'
import { commentQueryKeys, fieldNoteQueryKeys } from '../api'
import { useDeleteComment, useUpdateComment } from './index'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const showComments = [...commentQueryKeys.entity('show', 5), 'best']
const showFieldNotes = fieldNoteQueryKeys.show(5)
const venueComments = [...commentQueryKeys.entity('venue', 5), 'best']
const seeded: QueryKey[] = [showComments, showFieldNotes, venueComments]

function seededClient() {
  const client = clientWithAppDefaults()
  for (const key of seeded) {
    client.setQueryData(key, { comments: [], total: 0, has_more: false })
  }
  return client
}

describe('what a comment edit or delete refetches', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    mockApiRequest.mockResolvedValue({})
  })

  it.each([
    [
      'useUpdateComment',
      mutationCase(useUpdateComment, {
        commentId: 1,
        body: 'edited',
        entityType: 'show',
        entityId: 5,
      }),
    ],
    [
      'useDeleteComment',
      mutationCase(useDeleteComment, { commentId: 1, entityType: 'show', entityId: 5 }),
    ],
  ])('%s on a show refetches its comments and its field notes', async (_name, run) => {
    const client = seededClient()
    await run(client)
    expect(invalidatedKeys(client, seeded)).toEqual([showComments, showFieldNotes])
  })

  it.each([
    [
      'useUpdateComment',
      mutationCase(useUpdateComment, {
        commentId: 1,
        body: 'edited',
        entityType: 'venue',
        entityId: 5,
      }),
    ],
    [
      'useDeleteComment',
      mutationCase(useDeleteComment, { commentId: 1, entityType: 'venue', entityId: 5 }),
    ],
  ])('%s on a venue refetches only its comments', async (_name, run) => {
    const client = seededClient()
    await run(client)
    expect(invalidatedKeys(client, seeded)).toEqual([venueComments])
  })
})
