import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
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
})
