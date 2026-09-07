import { describe, it, expect } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/mocks/server'
import { TEST_API_BASE } from '@/test/mocks/handlers'
import { createWrapper } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import {
  useScenes,
  useSceneDetail,
  useSceneArtists,
  useSceneCollections,
} from './useScenes'

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

  // The hook's page-retention rule reads the SLUG back out of the previous
  // query's key by position, so it retains across a limit change and drops
  // across a scene change. Reordering `queryKeys.scenes.artists` would not fail
  // to compile and would not fail any render test — it would quietly start
  // comparing a limit to a slug, and the /atlas preview would paint the
  // previous scene's bands. This is the assertion that catches that.
  it('keeps the slug where the retention rule reads it in the artists key', () => {
    expect(queryKeys.scenes.artists('phoenix-az', 180, 10)[2]).toBe('phoenix-az')
  })
})

describe('useSceneCollections', () => {
  function stubCollections() {
    let capturedUrl = ''
    server.use(
      http.get(`${TEST_API_BASE}/scenes/:slug/collections`, ({ request }) => {
        capturedUrl = request.url
        return HttpResponse.json({ collections: [] })
      })
    )
    return () => capturedUrl
  }

  // The backend's own default owns the cap, so a caller that asks for nothing
  // must send nothing.
  it('fetches the scene collections rail and sends no limit of its own', async () => {
    const url = stubCollections()

    const { result } = renderHook(
      () => useSceneCollections({ slug: 'phoenix-az' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const requested = new URL(url())
    expect(requested.pathname).toBe('/scenes/phoenix-az/collections')
    expect(requested.searchParams.has('limit')).toBe(false)
    expect(result.current.data?.collections).toEqual([])
  })

  it('sends the limit the caller asks for', async () => {
    const url = stubCollections()

    const { result } = renderHook(
      () => useSceneCollections({ slug: 'phoenix-az', limit: 3 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(new URL(url()).searchParams.get('limit')).toBe('3')
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useSceneCollections({ slug: '' }), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  // The page caches every scene query by SLUG, never by the numeric scene id
  // this endpoint's rows carry (the PSY-1109 key-drift class). The limit is in
  // the key because it changes WHICH collections come back, and the leading
  // 'scenes' is what prefix-matched invalidation reaches.
  it('keys by slug and limit under the scenes prefix', () => {
    expect(queryKeys.scenes.collections('phoenix-az')).toEqual([
      'scenes',
      'collections',
      'phoenix-az',
      undefined,
    ])
    expect(queryKeys.scenes.collections('phoenix-az', 3)).toEqual([
      'scenes',
      'collections',
      'phoenix-az',
      3,
    ])
  })
})
