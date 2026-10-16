import { describe, it, expect, vi } from 'vitest'
import { hashKey, useQuery } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import { clientWithAppDefaults, deferred } from '@/test/inflightOrdering'
import { refetchAfterMutation } from './refetchAfterMutation'

describe('refetchAfterMutation', () => {
  it('starts a new request when the first fetch is still in flight, and discards the late response', async () => {
    const client = clientWithAppDefaults()
    const first = deferred<string>()
    const queryFn = vi
      .fn<() => Promise<string>>()
      .mockReturnValueOnce(first.promise)
      .mockResolvedValueOnce('after write')

    const { result } = renderHook(
      () => useQuery({ queryKey: ['comments', 'venue', 42, 'best'], queryFn }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(1))

    await act(() => refetchAfterMutation(client, ['comments', 'venue', 42]))
    await act(async () => {
      first.resolve('before write')
      await first.promise
    })

    await waitFor(() => expect(result.current.data).toBe('after write'))
    expect(queryFn).toHaveBeenCalledTimes(2)
  })

  it('keeps showing existing data while it refetches', async () => {
    const client = clientWithAppDefaults()
    const second = deferred<string>()
    const queryFn = vi
      .fn<() => Promise<string>>()
      .mockResolvedValueOnce('before write')
      .mockReturnValueOnce(second.promise)

    const { result } = renderHook(
      () => useQuery({ queryKey: ['comments', 'venue', 42, 'best'], queryFn }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(result.current.data).toBe('before write'))

    act(() => {
      void refetchAfterMutation(client, ['comments', 'venue', 42])
    })
    await waitFor(() => expect(queryFn).toHaveBeenCalledTimes(2))
    expect(result.current.data).toBe('before write')

    await act(async () => {
      second.resolve('after write')
      await second.promise
    })
    await waitFor(() => expect(result.current.data).toBe('after write'))
  })

  it('refetches every key it is given, each over its own in-flight first fetch', async () => {
    const client = clientWithAppDefaults()
    const firstList = deferred<string>()
    const firstDetail = deferred<string>()
    const listFn = vi
      .fn<() => Promise<string>>()
      .mockReturnValueOnce(firstList.promise)
      .mockResolvedValueOnce('list after write')
    const detailFn = vi
      .fn<() => Promise<string>>()
      .mockReturnValueOnce(firstDetail.promise)
      .mockResolvedValueOnce('detail after write')

    const { result } = renderHook(
      () => ({
        list: useQuery({ queryKey: ['requests', 'list'], queryFn: listFn }),
        detail: useQuery({
          queryKey: ['requests', 'detail', 7],
          queryFn: detailFn,
        }),
      }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => {
      expect(listFn).toHaveBeenCalledTimes(1)
      expect(detailFn).toHaveBeenCalledTimes(1)
    })

    await act(() =>
      refetchAfterMutation(
        client,
        ['requests', 'list'],
        ['requests', 'detail', 7]
      )
    )
    await act(async () => {
      firstList.resolve('list before write')
      firstDetail.resolve('detail before write')
      await Promise.all([firstList.promise, firstDetail.promise])
    })

    await waitFor(() => {
      expect(result.current.list.data).toBe('list after write')
      expect(result.current.detail.data).toBe('detail after write')
    })
    expect(listFn).toHaveBeenCalledTimes(2)
    expect(detailFn).toHaveBeenCalledTimes(2)
  })

  it('takes filters for a target a key cannot describe', async () => {
    const client = clientWithAppDefaults()
    const kept = ['collections', 'detail', 'new-one']
    const refetched = ['collections', 'list']
    client.setQueryData(kept, 'written by the caller')
    client.setQueryData(refetched, 'before write')

    await refetchAfterMutation(client, {
      queryKey: ['collections'],
      predicate: query => query.queryHash !== hashKey(kept),
    })

    expect(client.getQueryState(kept)?.isInvalidated).toBe(false)
    expect(client.getQueryState(refetched)?.isInvalidated).toBe(true)
  })

  it('cancels every key before it invalidates any', async () => {
    const client = clientWithAppDefaults()
    const calls: string[] = []
    vi.spyOn(client, 'cancelQueries').mockImplementation(async (filters) => {
      calls.push(`cancel ${JSON.stringify(filters?.queryKey)}`)
    })
    vi.spyOn(client, 'invalidateQueries').mockImplementation(async (filters) => {
      calls.push(`invalidate ${JSON.stringify(filters?.queryKey)}`)
    })

    await refetchAfterMutation(client, ['collections'], ['collections', 'my'])

    expect(calls).toEqual([
      'cancel ["collections"]',
      'cancel ["collections","my"]',
      'invalidate ["collections"]',
      'invalidate ["collections","my"]',
    ])
  })
})
