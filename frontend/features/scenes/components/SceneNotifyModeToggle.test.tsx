import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { renderWithProviders } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'

// The optimistic-update/rollback tests seed cache data at a query key with no
// active observer (useFollowStatus is mocked, so nothing `useQuery`s that
// key). The shared test QueryClient uses `gcTime: 0`, which garbage-collects
// such observer-less data immediately — before the mutation ever runs. Give
// those tests their own client with a real gcTime so the seeded cache survives.
function createCacheInspectableQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 5 * 60 * 1000 },
      mutations: { retry: false },
    },
  })
}

const mockUseFollowStatus = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true, user: { id: 42 } }),
}))
vi.mock('@/lib/hooks/common/useFollow', () => ({
  useFollowStatus: (entityType: string, entityId: number | string) =>
    mockUseFollowStatus(entityType, entityId),
}))

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

import { SceneNotifyModeToggle } from './SceneNotifyModeToggle'

describe('SceneNotifyModeToggle (PSY-1341)', () => {
  beforeEach(() => {
    mockUseFollowStatus.mockReset()
    mockApiRequest.mockReset()
    mockApiRequest.mockResolvedValue({ success: true })
  })

  it('renders nothing until the scene is followed', () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: false },
    })
    renderWithProviders(<SceneNotifyModeToggle slug="phoenix-az" />)
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
  })

  it('marks the stored mode checked, defaulting to all', () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: true },
    })
    renderWithProviders(<SceneNotifyModeToggle slug="phoenix-az" />)
    expect(screen.getByRole('radio', { name: 'All shows' })).toHaveAttribute(
      'aria-checked',
      'true'
    )
    expect(
      screen.getByRole('radio', { name: 'Bands I follow' })
    ).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByRole('radio', { name: 'Off' })).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })

  it('marks off checked when the stored mode is off (PSY-1466/PSY-1468)', () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: true, notify_mode: 'off' },
    })
    renderWithProviders(<SceneNotifyModeToggle slug="phoenix-az" />)
    expect(screen.getByRole('radio', { name: 'Off' })).toHaveAttribute(
      'aria-checked',
      'true'
    )
    expect(screen.getByRole('radio', { name: 'All shows' })).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })

  it('re-POSTs the follow with the picked mode', async () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: true, notify_mode: 'all' },
    })
    renderWithProviders(<SceneNotifyModeToggle slug="phoenix-az" />)
    fireEvent.click(screen.getByRole('radio', { name: 'Bands I follow' }))

    // useMutation runs the mutationFn in a microtask.
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))
    const [url, opts] = mockApiRequest.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/scenes/phoenix-az/follow')
    expect(opts.method).toBe('POST')
    expect(JSON.parse(opts.body as string)).toEqual({
      notify_mode: 'followed_bands_only',
    })
  })

  it('re-POSTs the follow with off when off is picked', async () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: true, notify_mode: 'all' },
    })
    renderWithProviders(<SceneNotifyModeToggle slug="phoenix-az" />)
    fireEvent.click(screen.getByRole('radio', { name: 'Off' }))

    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))
    const [url, opts] = mockApiRequest.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/scenes/phoenix-az/follow')
    expect(opts.method).toBe('POST')
    expect(JSON.parse(opts.body as string)).toEqual({ notify_mode: 'off' })
  })

  it('does not re-POST when the current mode is clicked', () => {
    mockUseFollowStatus.mockReturnValue({
      data: {
        follower_count: 3,
        is_following: true,
        notify_mode: 'followed_bands_only',
      },
    })
    renderWithProviders(<SceneNotifyModeToggle slug="phoenix-az" />)
    fireEvent.click(screen.getByRole('radio', { name: 'Bands I follow' }))
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('optimistically writes the picked mode into the follow-status cache', async () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: true, notify_mode: 'all' },
    })
    let resolveRequest: (value: { success: boolean }) => void = () => {}
    mockApiRequest.mockImplementation(
      () =>
        new Promise(resolve => {
          resolveRequest = resolve
        })
    )
    const { queryClient } = renderWithProviders(
      <SceneNotifyModeToggle slug="phoenix-az" />,
      { queryClient: createCacheInspectableQueryClient() }
    )
    const key = queryKeys.follows.entity('scenes', 'phoenix-az', 42)
    queryClient.setQueryData(key, {
      follower_count: 3,
      is_following: true,
      notify_mode: 'all',
    })

    fireEvent.click(screen.getByRole('radio', { name: 'Off' }))

    await waitFor(() =>
      expect(queryClient.getQueryData(key)).toMatchObject({
        notify_mode: 'off',
      })
    )

    resolveRequest({ success: true })
  })

  it('rolls back to the prior mode when the update fails', async () => {
    mockUseFollowStatus.mockReturnValue({
      data: { follower_count: 3, is_following: true, notify_mode: 'all' },
    })
    mockApiRequest.mockRejectedValueOnce(new Error('network error'))
    const { queryClient } = renderWithProviders(
      <SceneNotifyModeToggle slug="phoenix-az" />,
      { queryClient: createCacheInspectableQueryClient() }
    )
    const key = queryKeys.follows.entity('scenes', 'phoenix-az', 42)
    queryClient.setQueryData(key, {
      follower_count: 3,
      is_following: true,
      notify_mode: 'all',
    })

    fireEvent.click(screen.getByRole('radio', { name: 'Off' }))

    // The optimistic write lands then the rejected mutation's onError rolls
    // the cache back — assert on the settled (rolled-back) end state, since
    // the mocked rejection resolves too fast for waitFor's polling interval
    // to reliably observe the transient optimistic value in between.
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(1))
    await waitFor(() =>
      expect(queryClient.getQueryData(key)).toMatchObject({
        notify_mode: 'all',
      })
    )
  })
})
