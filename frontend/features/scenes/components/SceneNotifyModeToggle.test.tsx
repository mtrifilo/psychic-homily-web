import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

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
})
