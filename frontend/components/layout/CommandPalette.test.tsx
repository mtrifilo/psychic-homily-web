import React from 'react'
import { describe, it, expect, vi, beforeEach, beforeAll } from 'vitest'
import { screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { CommandPalette } from './CommandPalette'

// jsdom does not implement scrollIntoView
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

// Mock next/navigation
const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
}))

// Mock AuthContext
const mockAuthContext = {
  user: null as { id: string; email: string; is_admin?: boolean } | null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
  setUser: vi.fn(),
  setError: vi.fn(),
  clearError: vi.fn(),
  logout: vi.fn(),
}
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext,
}))

// Mock the entity search hook to avoid real API calls in basic tests
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => ({
    data: {
      artists: [],
      venues: [],
      releases: [],
      labels: [],
      festivals: [],
    },
    isSearching: false,
    totalResults: 0,
    isFetching: false,
  }),
}))

describe('CommandPalette', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockAuthContext.user = null
    mockAuthContext.isAuthenticated = false
  })

  it('should open on Cmd+K', async () => {
    renderWithProviders(<CommandPalette />)

    // Dialog should not be visible initially
    expect(screen.queryByPlaceholderText('Search entities or go to page...')).not.toBeInTheDocument()

    // Press Cmd+K
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    // Dialog should be visible
    expect(screen.getByPlaceholderText('Search entities or go to page...')).toBeInTheDocument()
  })

  it('should show public pages for unauthenticated users', async () => {
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    // Public pages visible
    expect(screen.getByText('Shows')).toBeInTheDocument()
    expect(screen.getByText('Artists')).toBeInTheDocument()
    expect(screen.getByText('Venues')).toBeInTheDocument()
    expect(screen.getByText('Blog')).toBeInTheDocument()
    expect(screen.getByText('DJ Sets')).toBeInTheDocument()
    expect(screen.getByText('My Submissions')).toBeInTheDocument()

    // Auth-only pages hidden
    expect(screen.queryByText('Library')).not.toBeInTheDocument()
    expect(screen.queryByText('Settings')).not.toBeInTheDocument()
    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
  })

  it('should show auth pages for authenticated users', async () => {
    mockAuthContext.user = { id: '1', email: 'test@test.com' }
    mockAuthContext.isAuthenticated = true

    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.getByText('Library')).toBeInTheDocument()
    expect(screen.getByText('Settings')).toBeInTheDocument()
    // Admin should still be hidden for non-admin
    expect(screen.queryByText('Admin')).not.toBeInTheDocument()
  })

  it('should not have a standalone "Collection" entry (merged into Library)', async () => {
    mockAuthContext.user = { id: '1', email: 'test@test.com' }
    mockAuthContext.isAuthenticated = true

    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    // "Collections" (plural) exists from the Discover section; "Collection" (singular) should not.
    expect(screen.getByText('Collections')).toBeInTheDocument()
    expect(screen.queryByText('Collection')).not.toBeInTheDocument()
  })

  it('should show admin page for admin users', async () => {
    mockAuthContext.user = { id: '1', email: 'admin@test.com', is_admin: true }
    mockAuthContext.isAuthenticated = true

    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.getByText('Admin')).toBeInTheDocument()
  })

  it('should navigate on item selection', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    const showsItem = screen.getByText('Shows')
    await user.click(showsItem)

    expect(mockPush).toHaveBeenCalledWith('/shows')
  })

  it('should show recent searches after selection', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    // Open and select Shows
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    await user.click(screen.getByText('Shows'))

    // Reopen -- recent searches should appear
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.getByText('Recent')).toBeInTheDocument()
  })

  it('should close on Escape', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.getByPlaceholderText('Search entities or go to page...')).toBeInTheDocument()

    await user.keyboard('{Escape}')

    expect(screen.queryByPlaceholderText('Search entities or go to page...')).not.toBeInTheDocument()
  })

  it('should show keyboard navigation hints', async () => {
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.getByText('navigate')).toBeInTheDocument()
    expect(screen.getByText('select')).toBeInTheDocument()
    expect(screen.getByText('close')).toBeInTheDocument()
  })

  it('should open via custom event (openCommandPalette)', async () => {
    renderWithProviders(<CommandPalette />)

    act(() => {
      window.dispatchEvent(new Event('open-command-palette'))
    })

    expect(screen.getByPlaceholderText('Search entities or go to page...')).toBeInTheDocument()
  })
})
