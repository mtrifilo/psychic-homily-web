import { describe, it, expect } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/mocks/server'
import { TEST_API_BASE } from '@/test/mocks/handlers'
import { createWrapper } from '@/test/utils'
import { useScenes, useSceneDetail, useSceneArtists } from './useScenes'

describe('useScenes', () => {
  it('fetches scene list', async () => {
    const { result } = renderHook(() => useScenes(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.scenes).toHaveLength(2)
    expect(result.current.data?.scenes[0].slug).toBe('phoenix-az')
  })

  it('handles empty scenes', async () => {
    server.use(
      http.get(`${TEST_API_BASE}/scenes`, () => {
        return HttpResponse.json({ scenes: [], count: 0 })
      })
    )

    const { result } = renderHook(() => useScenes(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.scenes).toEqual([])
  })

  it('handles API errors', async () => {
    server.use(
      http.get(`${TEST_API_BASE}/scenes`, () => {
        return HttpResponse.json(
          { message: 'Internal server error' },
          { status: 500 }
        )
      })
    )

    const { result } = renderHook(() => useScenes(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useSceneDetail', () => {
  it('fetches a scene by slug', async () => {
    const { result } = renderHook(() => useSceneDetail('phoenix-az'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.slug).toBe('phoenix-az')
    expect(result.current.data?.stats.venue_count).toBe(12)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useSceneDetail(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useSceneArtists', () => {
  it('fetches scene artists with default params', async () => {
    const { result } = renderHook(
      () => useSceneArtists({ slug: 'phoenix-az' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.artists).toBeDefined()
    expect(result.current.data?.total).toBe(3)
  })

  it('passes query parameters to the endpoint', async () => {
    // Override handler to capture and verify query params
    let capturedUrl = ''
    server.use(
      http.get(`${TEST_API_BASE}/scenes/:slug/artists`, ({ request }) => {
        capturedUrl = request.url
        return HttpResponse.json({ artists: [], total: 0 })
      })
    )

    const { result } = renderHook(
      () =>
        useSceneArtists({
          slug: 'phoenix-az',
          period: 30,
          limit: 50,
          offset: 10,
        }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const url = new URL(capturedUrl)
    expect(url.searchParams.get('period')).toBe('30')
    expect(url.searchParams.get('limit')).toBe('50')
    expect(url.searchParams.get('offset')).toBe('10')
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(
      () => useSceneArtists({ slug: '' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})
