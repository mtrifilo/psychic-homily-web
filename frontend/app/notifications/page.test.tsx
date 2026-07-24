import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { NotificationLogEntry } from '@/features/notifications'

// ─── Mocks ─────────────────────────────────────────────

const mockUseUserNotifications = vi.fn()
const mockMarkReadMutate = vi.fn()
const mockUseMarkRead = vi.fn(() => ({
  mutate: mockMarkReadMutate,
  isPending: false,
  isError: false,
  error: null as Error | null,
}))
const mockAuthContext = vi.fn(() => ({
  isAuthenticated: true,
  isLoading: false,
  user: { id: 1 },
}))
const mockRedirect = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

vi.mock('next/navigation', async importOriginal => ({
  ...(await importOriginal<object>()),
  redirect: (...args: unknown[]) => mockRedirect(...args),
}))

vi.mock('@/features/notifications/hooks', async importOriginal => ({
  ...(await importOriginal<object>()),
  useUserNotifications: () => mockUseUserNotifications(),
  useMarkNotificationsRead: () => mockUseMarkRead(),
}))

import NotificationInboxPage from './page'

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

function mockData(
  notifications: NotificationLogEntry[],
  unread_count: number
) {
  mockUseUserNotifications.mockReturnValue({
    data: { notifications, unread_count },
    isLoading: false,
    isError: false,
    error: null,
  })
}

describe('NotificationInboxPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: 1 },
    })
  })

  it('mounting the page marks NOTHING read (PSY-1513 policy reversal)', () => {
    mockData([commentEntry()], 1)
    render(<NotificationInboxPage />)
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(mockMarkReadMutate).not.toHaveBeenCalled()
  })

  it('shows the numeric unread count next to the title', () => {
    mockData([commentEntry()], 4)
    render(<NotificationInboxPage />)
    expect(screen.getByText('4 unread')).toBeInTheDocument()
  })

  it('defaults to the unread view: unread rows first, read rows under EARLIER, dimmed', () => {
    const readRow = commentEntry({
      id: 2,
      commenter_name: 'bob',
      read_at: new Date().toISOString(),
    })
    mockData([readRow, commentEntry()], 1)
    render(<NotificationInboxPage />)
    expect(screen.getByText('Earlier')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /bob/i })).toHaveClass('opacity-60')
    expect(screen.getByRole('link', { name: /alice/i })).not.toHaveClass(
      'opacity-60'
    )
  })

  it('[all] shows the interleaved history without the EARLIER section', async () => {
    const readRow = commentEntry({
      id: 2,
      commenter_name: 'bob',
      read_at: new Date().toISOString(),
    })
    mockData([readRow, commentEntry()], 1)
    const user = userEvent.setup()
    render(<NotificationInboxPage />)
    await user.click(screen.getByRole('button', { name: /show all/i }))
    expect(screen.queryByText('Earlier')).not.toBeInTheDocument()
    expect(screen.getByText('bob')).toBeInTheDocument()
    expect(screen.getByText('alice')).toBeInTheDocument()
  })

  it('clicking a row fires a scoped mark-read with that row id', async () => {
    const entry = commentEntry()
    mockData([entry], 1)
    const user = userEvent.setup()
    render(<NotificationInboxPage />)
    await user.click(screen.getByRole('link', { name: /alice/i }))
    expect(mockMarkReadMutate).toHaveBeenCalledWith([entry.id])
  })

  it('the [mark read] affordance fires a scoped mark-read', async () => {
    const entry = commentEntry()
    mockData([entry], 1)
    const user = userEvent.setup()
    render(<NotificationInboxPage />)
    await user.click(screen.getByRole('button', { name: '[mark read]' }))
    expect(mockMarkReadMutate).toHaveBeenCalledWith([entry.id])
  })

  it('[Catch up] fires mark-all (no ids)', async () => {
    mockData([commentEntry()], 1)
    const user = userEvent.setup()
    render(<NotificationInboxPage />)
    await user.click(screen.getByRole('button', { name: /catch up/i }))
    expect(mockMarkReadMutate).toHaveBeenCalledWith(undefined)
  })

  it('hides [Catch up] when there is nothing unread', () => {
    mockData([commentEntry({ read_at: new Date().toISOString() })], 0)
    render(<NotificationInboxPage />)
    expect(
      screen.queryByRole('button', { name: /catch up/i })
    ).not.toBeInTheDocument()
  })

  it('shows the caught-up empty state in the unread view when everything is read', () => {
    mockData([commentEntry({ read_at: new Date().toISOString() })], 0)
    render(<NotificationInboxPage />)
    expect(screen.getByText(/all caught up/i)).toBeInTheDocument()
    expect(screen.getByText('Earlier')).toBeInTheDocument()
  })

  it('surfaces mark-read mutation errors inline', () => {
    mockData([commentEntry()], 1)
    mockUseMarkRead.mockReturnValue({
      mutate: mockMarkReadMutate,
      isPending: false,
      isError: true,
      error: new Error('boom'),
    })
    render(<NotificationInboxPage />)
    expect(
      screen.getByText(/couldn't mark notifications read/i)
    ).toBeInTheDocument()
  })

  it('redirects to /auth when not authenticated', () => {
    mockAuthContext.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null as never,
    })
    mockUseUserNotifications.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
    })
    render(<NotificationInboxPage />)
    expect(mockRedirect).toHaveBeenCalledWith('/auth')
  })
})
