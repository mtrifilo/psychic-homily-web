import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationLogEntry } from '../types'

// ─── Mocks ─────────────────────────────────────────────

const mockUseUserNotifications = vi.fn()
const mockMarkReadMutate = vi.fn()
const mockUseMarkRead = vi.fn(() => ({ mutate: mockMarkReadMutate }))
const mockAuthContext = vi.fn(() => ({
  isAuthenticated: true,
  isLoading: false,
  user: { id: 1 },
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

vi.mock('../hooks', () => ({
  useUserNotifications: () => mockUseUserNotifications(),
  useMarkNotificationsRead: () => mockUseMarkRead(),
}))

import { NotificationBell } from './NotificationBell'

function commentEntry(
  overrides: Partial<NotificationLogEntry> = {}
): NotificationLogEntry {
  return {
    id: 1,
    entity_type: 'comment_reply',
    entity_id: 100,
    channel: 'in_app',
    sent_at: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
    read_at: null,
    commenter_name: 'alice',
    comment_excerpt: 'reply body',
    comment_url: 'https://example.com/shows/x#comment-100',
    comment_entity_type: 'show',
    comment_entity_id: 1,
    comment_entity_name: 'A Show',
    ...overrides,
  }
}

function renderBell() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <NotificationBell />
    </QueryClientProvider>
  )
}

describe('NotificationBell', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: 1 },
    })
  })

  it('returns null when not authenticated', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null as never,
    })
    mockUseUserNotifications.mockReturnValue({
      data: undefined,
      isLoading: false,
    })
    const { container } = renderBell()
    expect(container.firstChild).toBeNull()
  })

  it('renders the bell button when authenticated with no unread', () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [], unread_count: 0 },
      isLoading: false,
    })
    renderBell()
    expect(screen.getByRole('button', { name: /notifications/i })).toBeInTheDocument()
  })

  it('renders the unread count badge when there are unread notifications', () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [commentEntry()], unread_count: 3 },
      isLoading: false,
    })
    renderBell()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /3 unread/i })
    ).toBeInTheDocument()
  })

  it('caps the badge at "9+" when unread > 9', () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [], unread_count: 47 },
      isLoading: false,
    })
    renderBell()
    expect(screen.getByText('9+')).toBeInTheDocument()
  })

  it('opens a popover with notification rows when clicked, and fires mark-read', async () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [commentEntry()], unread_count: 1 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByText('A Show')).toBeInTheDocument()

    // mark-read is fired on a 500ms delay after open
    await waitFor(
      () => {
        expect(mockMarkReadMutate).toHaveBeenCalledWith(undefined)
      },
      { timeout: 1000 }
    )
  })

  it('does not fire mark-read when there are no unread entries', async () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [], unread_count: 0 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    // Give time for any debounced mutation to (not) fire.
    await new Promise(r => setTimeout(r, 600))
    expect(mockMarkReadMutate).not.toHaveBeenCalled()
  })
})
