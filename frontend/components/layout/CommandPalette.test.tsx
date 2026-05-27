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
let mockPathname = '/'
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
  // PSY-366: usePathname drives the contextual "Explore graph" entries.
  usePathname: () => mockPathname,
}))

// Mock AuthContext
const mockAuthContext = {
  user: null as { id: string; email: string; is_admin?: boolean } | null,
  isAuthenticated: false,
  isLoading: false,
  error: null as Error | null,
  setUser: vi.fn(),
  setError: vi.fn(),
  clearError: vi.fn(),
  logout: vi.fn(),
}
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext,
}))

// Mock the entity search hook to avoid real API calls in basic tests.
// `mockEntitySearchResult` lets individual tests seed tag/entity results
// without refactoring module-level mocks.
type MockedEntitySearchData = {
  artists: unknown[]
  venues: unknown[]
  // PSY-372: shows are returned by useEntitySearch but the palette doesn't
  // surface them; included here so the mock matches the real shape.
  shows: unknown[]
  releases: unknown[]
  labels: unknown[]
  festivals: unknown[]
  tags: unknown[]
}
const emptyEntityData: MockedEntitySearchData = {
  artists: [],
  venues: [],
  shows: [],
  releases: [],
  labels: [],
  festivals: [],
  tags: [],
}
let mockEntitySearchResult: {
  data: MockedEntitySearchData
  isSearching: boolean
  totalResults: number
  isFetching: boolean
  /**
   * PSY-725: total-outage flag exposed by useEntitySearch. Defaults false
   * so existing tests don't accidentally surface the banner.
   */
  searchError: boolean
} = {
  data: emptyEntityData,
  isSearching: false,
  totalResults: 0,
  isFetching: false,
  searchError: false,
}
vi.mock('@/lib/hooks/common/useEntitySearch', () => ({
  useEntitySearch: () => mockEntitySearchResult,
  // PSY-725: consumers import the canonical banner copy from the same
  // module. Re-export the literal here so the mock fully replaces the real
  // module without forcing tests to assert against a stub value.
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE:
    'Search is temporarily unavailable. Try again in a moment.',
}))

describe('CommandPalette', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockAuthContext.user = null
    mockAuthContext.isAuthenticated = false
    mockEntitySearchResult = {
      data: emptyEntityData,
      isSearching: false,
      totalResults: 0,
      isFetching: false,
      searchError: false,
    }
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

  // Avoid false coverage: the previous Cmd+K test only checked the search
  // input was visible. Pinning the dialog role here ensures the palette
  // actually OPENED rather than something rendering the input incidentally.
  it('should open the dialog (role="dialog") on Cmd+K', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    await user.keyboard('{Meta>}k{/Meta}')

    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('should toggle closed on a second Cmd+K press', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    await user.keyboard('{Meta>}k{/Meta}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await user.keyboard('{Meta>}k{/Meta}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('clears the search query after closing and reopening', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    // Seed the input
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })
    const input = screen.getByPlaceholderText('Search entities or go to page...')
    await user.type(input, 'shoegaze')
    expect(input).toHaveValue('shoegaze')

    // Close + reopen — useEffect on `open` resets the query.
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })
    const reopened = screen.getByPlaceholderText('Search entities or go to page...')
    expect(reopened).toHaveValue('')
  })

  it('shows a Clear button that empties the query without closing the palette', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    const input = screen.getByPlaceholderText('Search entities or go to page...')
    await user.type(input, 'doom')
    expect(input).toHaveValue('doom')
    // Clear button only appears with a non-empty query
    const clearButton = screen.getByRole('button', { name: 'Clear search' })
    await user.click(clearButton)
    expect(input).toHaveValue('')
    // Dialog still open after clearing
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })

  it('clearing recent searches removes the Recent group from the palette', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    // Open + select to seed a recent search
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })
    await user.click(screen.getByText('Shows'))

    // Reopen — Recent group should now exist
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })
    expect(screen.getByText('Recent')).toBeInTheDocument()

    // Click the inline "Clear" button on the Recent header. cmdk renders
    // the heading itself with role="button" whose accessible name includes
    // child text ("Recent Clear"); the actual <button> is a child element,
    // so target it by text instead of role.
    await user.click(screen.getByText('Clear'))

    // Recent group disappears immediately
    expect(screen.queryByText('Recent')).not.toBeInTheDocument()
  })

  it('navigates to an entity result when an entity row is clicked', async () => {
    const user = userEvent.setup()
    mockEntitySearchResult = {
      data: {
        ...emptyEntityData,
        artists: [
          {
            id: 7,
            slug: 'sunn-o',
            name: 'Sunn O)))',
            subtitle: 'Seattle, WA',
            entityType: 'artist',
            href: '/artists/sunn-o',
          },
        ],
      },
      isSearching: false,
      totalResults: 1,
      isFetching: false,
      searchError: false,
    }

    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    const input = screen.getByPlaceholderText('Search entities or go to page...')
    await user.type(input, 'sun')

    const row = screen.getByText('Sunn O)))')
    await user.click(row)

    expect(mockPush).toHaveBeenCalledWith('/artists/sunn-o')
  })

  it('shows a loading spinner while isSearching is true', async () => {
    const user = userEvent.setup()
    mockEntitySearchResult = {
      data: emptyEntityData,
      isSearching: true,
      totalResults: 0,
      isFetching: true,
      searchError: false,
    }
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    const input = screen.getByPlaceholderText('Search entities or go to page...')
    await user.type(input, 'do')

    // Clear button is suppressed while searching; spinner replaces it.
    expect(screen.queryByRole('button', { name: 'Clear search' })).not.toBeInTheDocument()
  })
})

describe('CommandPalette — tag row official indicator (PSY-453)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockAuthContext.user = null
    mockAuthContext.isAuthenticated = false
  })

  it('renders the shared official indicator on official tag rows only', async () => {
    const user = userEvent.setup()
    mockEntitySearchResult = {
      data: {
        artists: [],
        venues: [],
        shows: [],
        releases: [],
        labels: [],
        festivals: [],
        tags: [
          {
            id: 1,
            slug: 'shoegaze',
            name: 'shoegaze',
            subtitle: 'Genre',
            entityType: 'tag',
            href: '/tags/shoegaze',
            isOfficial: true,
          },
          {
            id: 2,
            slug: 'dreampop',
            name: 'dreampop',
            subtitle: 'Genre',
            entityType: 'tag',
            href: '/tags/dreampop',
            isOfficial: false,
          },
        ],
      },
      isSearching: false,
      totalResults: 2,
      isFetching: false,
      searchError: false,
    }

    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    const input = screen.getByPlaceholderText('Search entities or go to page...')
    await user.type(input, 'sho')

    const markers = screen.getAllByRole('img', { name: 'Official tag' })
    expect(markers).toHaveLength(1)
    expect(markers[0]).toHaveAttribute('title', 'shoegaze (Official)')
  })
})

describe('CommandPalette — contextual Explore graph entries (PSY-366)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockAuthContext.user = null
    mockAuthContext.isAuthenticated = false
    mockPathname = '/'
    mockEntitySearchResult = {
      data: emptyEntityData,
      isSearching: false,
      totalResults: 0,
      isFetching: false,
      searchError: false,
    }
  })

  it('shows "Explore graph for this artist" on /artists/[slug]', async () => {
    mockPathname = '/artists/sunn-o'
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.getByText('Explore graph for this artist')).toBeInTheDocument()
  })

  it('does NOT show contextual graph entry on a non-entity page', async () => {
    // mockPathname stays at '/' from beforeEach.
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    expect(screen.queryByText('Explore graph for this artist')).not.toBeInTheDocument()
    expect(screen.queryByText('Explore graph for this venue')).not.toBeInTheDocument()
    expect(screen.queryByText('Explore graph for this scene')).not.toBeInTheDocument()
  })
})

// PSY-725: when every backing search endpoint fails, the hook flips its
// `searchError` flag and the palette has to swap the silent empty state for
// an explicit outage banner. Otherwise users read "no matches" and retype.
describe('CommandPalette — search outage banner (PSY-725)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockAuthContext.user = null
    mockAuthContext.isAuthenticated = false
    mockEntitySearchResult = {
      data: emptyEntityData,
      isSearching: false,
      totalResults: 0,
      isFetching: false,
      searchError: true,
    }
  })

  it('renders the InlineErrorBanner when searchError is true and query is 2+ chars', async () => {
    const user = userEvent.setup()
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    const input = screen.getByPlaceholderText('Search entities or go to page...')
    await user.type(input, 'doom')

    const banner = screen.getByTestId('entity-search-error-banner')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveTextContent(/Search is temporarily unavailable/i)
    // role="alert" is part of the banner contract — screen readers should
    // announce the outage immediately.
    expect(banner).toHaveAttribute('role', 'alert')
  })

  it('does NOT render the banner before the user types 2+ chars', async () => {
    renderWithProviders(<CommandPalette />)

    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true })
      )
    })

    // Palette open with searchError=true but no query yet — banner must
    // stay hidden so users see the static routes, not a stale outage flag.
    expect(screen.queryByTestId('entity-search-error-banner')).not.toBeInTheDocument()
  })
})
