import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { QueryKey } from '@tanstack/react-query'
import { queryKeys } from '@/lib/queryClient'
import { clientWithAppDefaults } from '@/test/inflightOrdering'
import { invalidatedKeys, mutationCase } from '@/test/mutationRefetchKeys'
import {
  useApproveFulfillment,
  useCloseRequest,
  useCreateRequest,
  useDeleteRequest,
  useFulfillRequest,
  useRejectFulfillment,
  useRemoveVoteRequest,
  useUpdateRequest,
  useVoteRequest,
} from './index'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const list = queryKeys.requests.list(undefined)
const detail = queryKeys.requests.detail(7)
const seeded: QueryKey[] = [list, detail]

function seededClient() {
  const client = clientWithAppDefaults()
  client.setQueryData(list, { requests: [], total: 0 })
  client.setQueryData(detail, {
    id: 7,
    upvotes: 0,
    downvotes: 0,
    vote_score: 0,
    user_vote: null,
  })
  return client
}

describe('what each request mutation refetches', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    mockApiRequest.mockResolvedValue({})
  })

  it.each([
    [
      'useCreateRequest',
      mutationCase(useCreateRequest, { title: 'Add', entity_type: 'artist' }),
    ],
    ['useUpdateRequest', mutationCase(useUpdateRequest, { requestId: 7, title: 'T' })],
    ['useDeleteRequest', mutationCase(useDeleteRequest, { requestId: 7 })],
    ['useVoteRequest', mutationCase(useVoteRequest, { requestId: 7, is_upvote: true })],
    ['useRemoveVoteRequest', mutationCase(useRemoveVoteRequest, { requestId: 7 })],
    ['useFulfillRequest', mutationCase(useFulfillRequest, { requestId: 7 })],
    ['useApproveFulfillment', mutationCase(useApproveFulfillment, { requestId: 7 })],
    ['useRejectFulfillment', mutationCase(useRejectFulfillment, { requestId: 7 })],
    ['useCloseRequest', mutationCase(useCloseRequest, { requestId: 7 })],
  ])('%s refetches the list and the detail', async (_name, run) => {
    const client = seededClient()
    await run(client)
    expect(invalidatedKeys(client, seeded)).toEqual(seeded)
  })
})
