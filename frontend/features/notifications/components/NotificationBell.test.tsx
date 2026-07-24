import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationLogEntry } from '../types'

// ─── Mocks ─────────────────────────────────────────────

const mockUseUserNotifications = vi.fn()
const mockMarkReadMutate = vi.fn()
const mockUseMarkRead = vi.fn(() => ({
  mutate: mockMarkReadMutate,
  isPending: false,
  isError: false,
}))
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

  it('renders the bell button with no badge when there are no unread', () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [], unread_count: 0 },
      isLoading: false,
    })
    renderBell()
    expect(screen.getByRole('button', { name: /notifications/i })).toBeInTheDocument()
    expect(screen.queryByTestId('notification-unread-badge')).not.toBeInTheDocument()
  })

  it('shows a numeric badge with the unread count (PSY-1513)', () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [commentEntry()], unread_count: 3 },
      isLoading: false,
    })
    renderBell()
    const badge = screen.getByTestId('notification-unread-badge')
    expect(badge).toHaveTextContent('3')
    expect(
      screen.getByRole('button', { name: /3 unread/i })
    ).toBeInTheDocument()
  })

  it('shows the exact count above 9 (no "9+" cap)', () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [], unread_count: 47 },
      isLoading: false,
    })
    renderBell()
    expect(screen.getByTestId('notification-unread-badge')).toHaveTextContent('47')
    expect(screen.queryByText('9+')).not.toBeInTheDocument()
  })

  it('opening the popover marks NOTHING read (PSY-1513 policy reversal)', async () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [commentEntry()], unread_count: 1 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByText('A Show')).toBeInTheDocument()

    // Old behavior fired mark-all after 500ms; give it time to (not) fire.
    await new Promise(r => setTimeout(r, 600))
    expect(mockMarkReadMutate).not.toHaveBeenCalled()
  })

  it('clicking a row fires a scoped mark-read with that row id', async () => {
    const entry = commentEntry()
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [entry], unread_count: 1 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    await user.click(screen.getByRole('link', { name: /alice/i }))
    expect(mockMarkReadMutate).toHaveBeenCalledWith([entry.id])
  })

  it('clicking an already-read row does not fire mark-read', async () => {
    const entry = commentEntry({ read_at: new Date().toISOString() })
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [entry], unread_count: 0 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    await user.click(screen.getByRole('link', { name: /alice/i }))
    expect(mockMarkReadMutate).not.toHaveBeenCalled()
  })

  it('[Catch up] fires mark-all (no ids)', async () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [commentEntry()], unread_count: 1 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    await user.click(
      screen.getByRole('button', { name: /catch up/i })
    )
    expect(mockMarkReadMutate).toHaveBeenCalledWith(undefined)
  })

  it('hides [Catch up] when there is nothing unread', async () => {
    mockUseUserNotifications.mockReturnValue({
      data: { notifications: [], unread_count: 0 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    expect(
      screen.queryByRole('button', { name: /catch up/i })
    ).not.toBeInTheDocument()
  })

  it('renders read rows under an EARLIER divider after unread rows', async () => {
    const unreadRow = commentEntry({ id: 1 })
    const readRow = commentEntry({
      id: 2,
      commenter_name: 'bob',
      read_at: new Date().toISOString(),
    })
    mockUseUserNotifications.mockReturnValue({
      // Server interleaves newest-first; the popover re-groups unread first.
      data: { notifications: [readRow, unreadRow], unread_count: 1 },
      isLoading: false,
    })
    const user = userEvent.setup()
    renderBell()
    await user.click(screen.getByRole('button', { name: /notifications/i }))
    expect(screen.getByText('Earlier')).toBeInTheDocument()
    const links = screen.getAllByRole('link')
    // First row link is the unread one (alice), read (bob) comes after.
    const rowText = links.map(l => l.textContent ?? '')
    expect(rowText.findIndex(t => t.includes('alice'))).toBeLessThan(
      rowText.findIndex(t => t.includes('bob'))
    )
  })
})
