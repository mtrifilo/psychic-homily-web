import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, fireEvent } from '@testing-library/react'
import { ContributionPrompt } from './ContributionPrompt'

// --- Mocks ---

const mockUseDataGaps = vi.fn()

vi.mock('../hooks/useDataGaps', () => ({
  useDataGaps: (...args: unknown[]) => mockUseDataGaps(...args),
}))

// --- localStorage mock ---

let store: Record<string, string> = {}

const localStorageMock = {
  getItem: vi.fn((key: string) => store[key] ?? null),
  setItem: vi.fn((key: string, value: string) => {
    store[key] = value
  }),
  removeItem: vi.fn((key: string) => {
    delete store[key]
  }),
  clear: vi.fn(() => {
    store = {}
  }),
  get length() {
    return Object.keys(store).length
  },
  key: vi.fn((_index: number) => null),
}

describe('ContributionPrompt', () => {
  const defaultProps = {
    entityType: 'artist' as const,
    entityId: 42,
    entitySlug: 'test-artist',
    isAuthenticated: true,
    onEditClick: vi.fn(),
  }

  beforeEach(() => {
    vi.clearAllMocks()
    store = {}
    // Re-apply implementations after clearAllMocks wipes them
    localStorageMock.getItem.mockImplementation((key: string) => store[key] ?? null)
    localStorageMock.setItem.mockImplementation((key: string, value: string) => {
      store[key] = value
    })
    Object.defineProperty(window, 'localStorage', {
      value: localStorageMock,
      writable: true,
    })
  })

  it('renders prompt when gaps exist', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [
          { field: 'bandcamp', label: 'Bandcamp URL', priority: 1 },
          { field: 'spotify', label: 'Spotify URL', priority: 2 },
        ],
      },
      isLoading: false,
    })

    render(<ContributionPrompt {...defaultProps} />)

    expect(screen.getByTestId('contribution-prompt')).toBeInTheDocument()
    expect(screen.getByText("Know this artist's Bandcamp?")).toBeInTheDocument()
    expect(screen.getByText('Add it')).toBeInTheDocument()
  })

  it('renders nothing when no gaps', () => {
    mockUseDataGaps.mockReturnValue({
      data: { gaps: [] },
      isLoading: false,
    })

    const { container } = render(<ContributionPrompt {...defaultProps} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders nothing when not authenticated', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'bandcamp', label: 'Bandcamp URL', priority: 1 }],
      },
      isLoading: false,
    })

    const { container } = render(
      <ContributionPrompt {...defaultProps} isAuthenticated={false} />
    )
    expect(container.firstChild).toBeNull()
  })

  it('dismiss button stores dismissal in localStorage', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'bandcamp', label: 'Bandcamp URL', priority: 1 }],
      },
      isLoading: false,
    })

    render(<ContributionPrompt {...defaultProps} />)

    const dismissBtn = screen.getByTestId('dismiss-prompt')
    fireEvent.click(dismissBtn)

    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      'dismissed-prompt-artist-42',
      'true'
    )
  })

  it('hides the prompt in the same render after the dismiss button is clicked', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'bandcamp', label: 'Bandcamp URL', priority: 1 }],
      },
      isLoading: false,
    })

    render(<ContributionPrompt {...defaultProps} />)
    expect(screen.getByTestId('contribution-prompt')).toBeInTheDocument()

    fireEvent.click(screen.getByTestId('dismiss-prompt'))

    expect(screen.queryByTestId('contribution-prompt')).not.toBeInTheDocument()
  })

  it('stays hidden after dismissal', () => {
    // Pre-set the dismissal in localStorage store
    store['dismissed-prompt-artist-42'] = 'true'

    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'bandcamp', label: 'Bandcamp URL', priority: 1 }],
      },
      isLoading: false,
    })

    const { container } = render(<ContributionPrompt {...defaultProps} />)
    expect(container.firstChild).toBeNull()
  })

  it('shows highest priority gap only', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [
          { field: 'bandcamp', label: 'Bandcamp URL', priority: 1 },
          { field: 'description', label: 'Description', priority: 7 },
        ],
      },
      isLoading: false,
    })

    render(<ContributionPrompt {...defaultProps} />)

    // Should show bandcamp (priority 1), not description
    expect(screen.getByText("Know this artist's Bandcamp?")).toBeInTheDocument()
    expect(screen.queryByText('Can you write a description?')).not.toBeInTheDocument()
  })

  it('calls onEditClick with the gap field when action button is clicked', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'website', label: 'Website', priority: 1 }],
      },
      isLoading: false,
    })

    render(<ContributionPrompt {...defaultProps} />)

    fireEvent.click(screen.getByText('Add it'))
    expect(defaultProps.onEditClick).toHaveBeenCalledTimes(1)
    expect(defaultProps.onEditClick).toHaveBeenCalledWith('website')
  })

  it('renders nothing while loading', () => {
    mockUseDataGaps.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    const { container } = render(<ContributionPrompt {...defaultProps} />)
    expect(container.firstChild).toBeNull()
  })

  it('shows correct prompt for venue fields', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'instagram', label: 'Instagram', priority: 2 }],
      },
      isLoading: false,
    })

    render(
      <ContributionPrompt
        {...defaultProps}
        entityType="venue"
        entityId={10}
        entitySlug="test-venue"
      />
    )

    expect(screen.getByText('Know the Instagram?')).toBeInTheDocument()
  })

  it('falls back to generic prompt text for fields not in the template map', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        // 'social_facebook' is not in promptTemplates — hits the fallback path.
        gaps: [{ field: 'social_facebook', label: 'Facebook', priority: 1 }],
      },
      isLoading: false,
    })

    render(<ContributionPrompt {...defaultProps} />)

    expect(screen.getByText("Help complete this artist's profile")).toBeInTheDocument()
  })

  it('shows correct prompt for festival flyer field', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'flyer_url', label: 'Flyer', priority: 2 }],
      },
      isLoading: false,
    })

    render(
      <ContributionPrompt
        {...defaultProps}
        entityType="festival"
        entityId={5}
        entitySlug="test-fest"
      />
    )

    expect(
      screen.getByText('Have a flyer for this festival?')
    ).toBeInTheDocument()
  })

  it('renders nothing when data is null', () => {
    mockUseDataGaps.mockReturnValue({
      data: null,
      isLoading: false,
    })

    const { container } = render(<ContributionPrompt {...defaultProps} />)
    expect(container.firstChild).toBeNull()
  })

  it('hides when a cross-tab storage event reports the prompt was dismissed', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'bandcamp', label: 'Bandcamp URL', priority: 1 }],
      },
      isLoading: false,
    })

    const { container } = render(<ContributionPrompt {...defaultProps} />)
    expect(screen.getByTestId('contribution-prompt')).toBeInTheDocument()

    // Another tab dismissed this prompt — the 'storage' event fires in
    // every other tab. useLocalStorageEnum's subscriber re-reads localStorage
    // and the prompt hides without any in-tab interaction.
    act(() => {
      store['dismissed-prompt-artist-42'] = 'true'
      window.dispatchEvent(
        new StorageEvent('storage', { key: 'dismissed-prompt-artist-42' })
      )
    })

    expect(container.firstChild).toBeNull()
  })

  it('passes the gating contract through to useDataGaps via the enabled flag', () => {
    mockUseDataGaps.mockReturnValue({
      data: { gaps: [] },
      isLoading: false,
    })

    // Unauthenticated viewer: enabled should be false so the network call is skipped.
    const { unmount } = render(
      <ContributionPrompt {...defaultProps} isAuthenticated={false} />
    )
    expect(mockUseDataGaps).toHaveBeenLastCalledWith(
      'artist',
      'test-artist',
      expect.objectContaining({ enabled: false })
    )
    unmount()

    // Authenticated viewer with no prior dismissal: enabled should be true.
    mockUseDataGaps.mockClear()
    render(<ContributionPrompt {...defaultProps} />)
    expect(mockUseDataGaps).toHaveBeenLastCalledWith(
      'artist',
      'test-artist',
      expect.objectContaining({ enabled: true })
    )

    // Pre-dismissed entity: enabled should be false so the gap fetch is skipped.
    mockUseDataGaps.mockClear()
    store['dismissed-prompt-venue-99'] = 'true'
    render(
      <ContributionPrompt
        {...defaultProps}
        entityType="venue"
        entityId={99}
        entitySlug="some-venue"
      />
    )
    expect(mockUseDataGaps).toHaveBeenLastCalledWith(
      'venue',
      'some-venue',
      expect.objectContaining({ enabled: false })
    )
  })

  it('keeps dismissals isolated per (entityType, entityId)', () => {
    mockUseDataGaps.mockReturnValue({
      data: {
        gaps: [{ field: 'bandcamp', label: 'Bandcamp URL', priority: 1 }],
      },
      isLoading: false,
    })

    // Artist 42 was dismissed in a prior session.
    store['dismissed-prompt-artist-42'] = 'true'

    // Artist 42 — should stay hidden.
    const { container: artistContainer } = render(
      <ContributionPrompt {...defaultProps} />
    )
    expect(artistContainer.firstChild).toBeNull()

    // Different entity (venue 10) — should still render.
    render(
      <ContributionPrompt
        {...defaultProps}
        entityType="venue"
        entityId={10}
        entitySlug="some-venue"
      />
    )
    expect(screen.getByTestId('contribution-prompt')).toBeInTheDocument()
  })
})
