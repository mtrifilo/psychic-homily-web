import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NotifyMeButton } from './NotifyMeButton'

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// Mock AuthContext
const mockAuthContext = vi.fn()
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock notification hooks
const mockQuickCreate = vi.fn()
const mockDeleteFilter = vi.fn()
const mockFilterCheck = vi.fn()

vi.mock('../hooks', () => ({
  useNotificationFilterCheck: (...args: unknown[]) => mockFilterCheck(...args),
  useQuickCreateFilter: () => mockQuickCreate(),
  useDeleteFilter: () => mockDeleteFilter(),
}))

describe('NotifyMeButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { id: '1' },
    })
    mockFilterCheck.mockReturnValue({
      data: undefined,
      hasFilter: false,
      isLoading: false,
      isSuccess: true,
    })
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
    mockDeleteFilter.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: false,
      error: null,
    })
  })

  it('renders "Notify me" for authenticated user without filter', () => {
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    expect(screen.getByText('Notify me')).toBeInTheDocument()
  })

  it('renders "Notifications on" when user has a matching filter', () => {
    mockFilterCheck.mockReturnValue({
      data: { id: 1, name: 'Filter' },
      hasFilter: true,
      isLoading: false,
      isSuccess: true,
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    expect(screen.getByText('Notifications on')).toBeInTheDocument()
  })

  it('redirects to auth when unauthenticated', async () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      user: null,
    })
    const user = userEvent.setup()
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    await user.click(screen.getByText('Notify me'))
    expect(mockPush).toHaveBeenCalledWith('/auth')
  })

  it('calls quickCreate.mutate when clicking notify without filter', async () => {
    const mutateFn = vi.fn()
    mockQuickCreate.mockReturnValue({
      mutate: mutateFn,
      isPending: false,
      isError: false,
      error: null,
    })
    const user = userEvent.setup()
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={42}
        entityName="Test Artist"
      />
    )
    await user.click(screen.getByText('Notify me'))
    expect(mutateFn).toHaveBeenCalledWith({ entityType: 'artist', entityId: 42 })
  })

  it('displays error message when quick-create mutation fails', () => {
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Network error'),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Network error')
  })

  it('displays error message when delete mutation fails', () => {
    mockFilterCheck.mockReturnValue({
      data: { id: 1, name: 'Filter' },
      hasFilter: true,
      isLoading: false,
      isSuccess: true,
    })
    mockDeleteFilter.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Failed to remove notification'),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Failed to remove notification')
  })

  it('displays fallback error message when error has no message', () => {
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error(''),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Failed to update notification. Please try again.')
  })

  it('displays error in compact mode when mutation fails', () => {
    mockQuickCreate.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
      isError: true,
      error: new Error('Server error'),
    })
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
        compact
      />
    )
    const alert = screen.getByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert).toHaveTextContent('Server error')
  })

  it('does not display error when no mutation has failed', () => {
    render(
      <NotifyMeButton
        entityType="artist"
        entityId={1}
        entityName="Test Artist"
      />
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
