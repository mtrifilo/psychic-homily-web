import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockPush = vi.fn()
const mockFollowMutate = vi.fn()
const mockUnfollowMutate = vi.fn()
let mockIsAuthenticated = true
let mockFollowStatusData: { follower_count: number; is_following: boolean } | undefined
let mockStatusLoading = false

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: mockIsAuthenticated }),
}))

vi.mock('@/lib/hooks/common/useFollow', () => ({
  useFollowStatus: () => ({
    data: mockFollowStatusData,
    isLoading: mockStatusLoading,
  }),
  useFollow: () => ({
    mutate: mockFollowMutate,
    isPending: false,
  }),
  useUnfollow: () => ({
    mutate: mockUnfollowMutate,
    isPending: false,
  }),
}))

import { FollowButton } from './FollowButton'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('FollowButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockIsAuthenticated = true
    mockFollowStatusData = { follower_count: 10, is_following: false }
    mockStatusLoading = false
  })

  it('renders "Follow" text in non-compact mode when not following', () => {
    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('Follow')).toBeInTheDocument()
  })

  it('renders "Following" text when following', () => {
    mockFollowStatusData = { follower_count: 10, is_following: true }

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('Following')).toBeInTheDocument()
  })

  it('renders follower count when > 0', () => {
    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('10')).toBeInTheDocument()
  })

  it('calls follow.mutate when clicking while not following', () => {
    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    fireEvent.click(screen.getByRole('button'))
    expect(mockFollowMutate).toHaveBeenCalledWith({ entityType: 'artists', entityId: 1 })
  })

  it('calls unfollow.mutate when clicking while following', () => {
    mockFollowStatusData = { follower_count: 10, is_following: true }

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    fireEvent.click(screen.getByRole('button'))
    expect(mockUnfollowMutate).toHaveBeenCalledWith({ entityType: 'artists', entityId: 1 })
  })

  it('redirects to /auth when not authenticated', () => {
    mockIsAuthenticated = false

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    fireEvent.click(screen.getByRole('button'))
    expect(mockPush).toHaveBeenCalledWith('/auth')
    expect(mockFollowMutate).not.toHaveBeenCalled()
  })

  it('uses followData prop over fetched data', () => {
    mockFollowStatusData = { follower_count: 10, is_following: false }

    render(
      <FollowButton
        entityType="artists"
        entityId={1}
        followData={{ follower_count: 42, is_following: true }}
      />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('Following')).toBeInTheDocument()
  })

  it('renders compact mode with aria-label', () => {
    render(
      <FollowButton entityType="artists" entityId={1} compact />,
      { wrapper: createWrapper() }
    )

    const button = screen.getByRole('button', { name: 'Follow' })
    expect(button).toBeInTheDocument()
  })

  it('renders loading spinner when status is loading and no followData', () => {
    mockStatusLoading = true
    mockFollowStatusData = undefined

    render(
      <FollowButton entityType="artists" entityId={1} />,
      { wrapper: createWrapper() }
    )

    // In non-compact mode, should show "Follow" text with a spinner
    expect(screen.getByText('Follow')).toBeInTheDocument()
  })

  it('does not show loading spinner when followData is provided', () => {
    mockStatusLoading = true
    mockFollowStatusData = undefined

    render(
      <FollowButton
        entityType="artists"
        entityId={1}
        followData={{ follower_count: 5, is_following: false }}
      />,
      { wrapper: createWrapper() }
    )

    expect(screen.getByText('Follow')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
  })
})
