import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapper, createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import {
  useReleaseSaveCountBatch,
  useReleaseSaveToggle,
  useSavedReleases,
} from './useSavedReleases'
import { releaseQueryKeys } from '../api'

describe('release save hooks', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('fetches the authenticated saved-release list with pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [], total: 0 })
    const { result } = renderHook(() => useSavedReleases(20, 40, 42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/saved-releases\?limit=20&offset=40$/),
      { method: 'GET' }
    )
  })

  it('waits for an authenticated user identity before reading the private list', () => {
    const { result } = renderHook(() => useSavedReleases(20, 0, undefined), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('posts release ids to the public batch status endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({
      saves: { '2': { save_count: 3, is_saved: false } },
    })
    const { result } = renderHook(
      () => useReleaseSaveCountBatch([2, 5], false),
      {
        wrapper: createWrapper(),
      }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/releases\/saves\/batch$/),
      { method: 'POST', body: JSON.stringify({ release_ids: [2, 5] }) }
    )
  })

  it('optimistically updates and then invalidates single and batch save state', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const singleKey = releaseQueryKeys.saveCount(7, true, 42)
    const batchKey = releaseQueryKeys.saveCountBatch([7], true, 42)
    const personalChartsKey = ['charts', 'personal', '42'] as const
    queryClient.setQueryData(singleKey, {
      release_id: 7,
      save_count: 2,
      is_saved: false,
    })
    queryClient.setQueryData(batchKey, {
      '7': { save_count: 2, is_saved: false },
    })
    queryClient.setQueryData(personalChartsKey, { first_activity_at: null })
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useReleaseSaveToggle(7, false, 42), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.toggle()
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/saved-releases\/7$/),
      { method: 'POST' }
    )
    expect(queryClient.getQueryState(singleKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(batchKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(personalChartsKey)?.isInvalidated).toBe(
      true
    )
  })

  it('rolls back only the failed release when a sibling toggle changed the same batch', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const batchKey = releaseQueryKeys.saveCountBatch([7, 8], true, 42)
    const personalChartsKey = ['charts', 'personal', '42'] as const
    queryClient.setQueryData(batchKey, {
      '7': { save_count: 2, is_saved: false },
      '8': { save_count: 1, is_saved: false },
    })
    queryClient.setQueryData(personalChartsKey, { first_activity_at: null })
    mockApiRequest.mockImplementationOnce(async () => {
      queryClient.setQueryData<
        Record<string, { save_count: number; is_saved: boolean }>
      >(batchKey, current => ({
        ...(current ?? {}),
        '8': { save_count: 2, is_saved: true },
      }))
      throw new Error('save failed')
    })

    const { result } = renderHook(() => useReleaseSaveToggle(7, false, 42), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await expect(result.current.toggle()).rejects.toThrow('save failed')
    const batch =
      queryClient.getQueryData<
        Record<string, { save_count: number; is_saved: boolean }>
      >(batchKey)
    expect(batch?.['7']).toEqual({ save_count: 2, is_saved: false })
    expect(batch?.['8']).toEqual({ save_count: 2, is_saved: true })
    await waitFor(() =>
      expect(queryClient.getQueryState(personalChartsKey)?.isInvalidated).toBe(
        true
      )
    )
  })

  it("does not patch another user's cached batch state", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const userBatch = releaseQueryKeys.saveCountBatch([7], true, 42)
    const otherUserBatch = releaseQueryKeys.saveCountBatch([7], true, 84)
    queryClient.setQueryData(userBatch, {
      '7': { save_count: 2, is_saved: false },
    })
    queryClient.setQueryData(otherUserBatch, {
      '7': { save_count: 2, is_saved: true },
    })
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useReleaseSaveToggle(7, false, 42), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.toggle()
    })

    expect(queryClient.getQueryData(otherUserBatch)).toEqual({
      '7': { save_count: 2, is_saved: true },
    })
    expect(queryClient.getQueryState(otherUserBatch)?.isInvalidated).toBe(false)
  })
})
