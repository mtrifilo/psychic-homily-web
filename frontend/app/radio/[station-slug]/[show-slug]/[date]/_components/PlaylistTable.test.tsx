import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PlaylistTable } from './PlaylistTable'
import { makeRadioPlay as makePlay } from '@/features/radio/lib/radioPlay.testutil'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

vi.mock('next/navigation', () => ({
  usePathname: () => '/radio/kexp/el-sonido/2026-06-02',
}))

type MockAuth = {
  user: { id: string } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}

const mockAuthContext = vi.fn<() => MockAuth>(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

vi.mock('@/features/auth', () => ({
  LoginPromptDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="login-prompt">Login Prompt</div> : null,
}))

const mockOwnPending = vi.fn()
const mockCreateMutate = vi.fn()

vi.mock('@/features/radio/hooks/usePlayMatchSuggestions', () => ({
  useOwnPlayMatchSuggestion: (...args: unknown[]) => mockOwnPending(...args),
  useCreatePlayMatchSuggestion: () => ({
    mutate: mockCreateMutate,
    isPending: false,
  }),
}))

vi.mock('@/features/artists', () => ({
  ArtistSearch: ({
    onSelect,
  }: {
    onSelect?: (artist: { id: number; name: string }) => void
  }) => (
    <button
      type="button"
      data-testid="mock-artist-search"
      onClick={() => onSelect?.({ id: 99, name: 'CAN' })}
    >
      Pick CAN
    </button>
  ),
}))


describe('PlaylistTable', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
    mockOwnPending.mockReturnValue({ data: null, isLoading: false })
  })

  it('renders a matched artist as a link with the filled dot', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ artist_id: 5, artist_slug: 'can' })]}
      />
    )
    expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
      'href',
      '/artists/can'
    )
    expect(screen.getAllByText('●')).toHaveLength(2)
  })

  it('renders an unmatched artist as plain text with the open dot', () => {
    render(<PlaylistTable plays={[makePlay({ artist_name: 'The Tweeters' })]} />)
    const artist = screen.getByText('The Tweeters')
    expect(artist.closest('a')).toBeNull()
    expect(screen.getAllByText('○')).toHaveLength(2)
  })

  it('renders track, album, label, and year', () => {
    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.getByText('Mother Sky')).toBeInTheDocument()
    expect(screen.getByText('Soundtracks')).toBeInTheDocument()
    expect(screen.getByText('United Artists')).toBeInTheDocument()
    expect(screen.getByText('1970')).toBeInTheDocument()
  })

  it('links the label when label_slug is present', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ label_id: 3, label_slug: 'united-artists' })]}
      />
    )
    expect(screen.getByRole('link', { name: 'United Artists' })).toHaveAttribute(
      'href',
      '/labels/united-artists'
    )
  })

  it('renders the TIME cell from air_timestamp and leaves it blank when null', () => {
    render(
      <PlaylistTable
        plays={[
          makePlay({ id: 1, air_timestamp: '2026-06-02T21:02:00' }),
          makePlay({ id: 2, artist_name: 'Neu!', air_timestamp: null }),
        ]}
      />
    )
    expect(screen.getByText('9:02 PM')).toBeInTheDocument()
    const timeCells = document.querySelectorAll('tbody td:first-child')
    expect(timeCells).toHaveLength(2)
    expect(timeCells[1].textContent).toBe('')
  })

  it('keeps rows in playlist order', () => {
    render(
      <PlaylistTable
        plays={[
          makePlay({ id: 1, position: 1, artist_name: 'CAN' }),
          makePlay({ id: 2, position: 2, artist_name: 'Neu!' }),
          makePlay({ id: 3, position: 3, artist_name: 'Harmonia' }),
        ]}
      />
    )
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows[0]).toHaveTextContent('CAN')
    expect(rows[1]).toHaveTextContent('Neu!')
    expect(rows[2]).toHaveTextContent('Harmonia')
  })

  it('renders LIVE, NEW, rotation, and REQ badges in the NOTES column', () => {
    render(
      <PlaylistTable
        plays={[
          makePlay({
            is_live_performance: true,
            is_new: true,
            rotation_status: 'recommended_new',
            is_request: true,
          }),
        ]}
      />
    )
    expect(screen.getByText('LIVE')).toBeInTheDocument()
    expect(screen.getByText('NEW')).toBeInTheDocument()
    expect(screen.getByText('REC NEW')).toBeInTheDocument()
    expect(screen.getByText('REQ')).toBeInTheDocument()
  })

  it('does not render a rotation tag for library rotation', () => {
    render(<PlaylistTable plays={[makePlay({ rotation_status: 'library' })]} />)
    expect(screen.queryByText('LIBRARY')).not.toBeInTheDocument()
  })

  it('renders a dj_comment as an indented sub-row under its track', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ dj_comment: 'recorded in Forst — RIP Michael Rother' })]}
      />
    )
    const comment = screen.getByText('recorded in Forst — RIP Michael Rother')
    expect(comment).toBeInTheDocument()
    const commentRow = comment.closest('tr')
    expect(commentRow).not.toBeNull()
    expect(commentRow).not.toHaveTextContent('Mother Sky')
  })

  it('does not render a comment sub-row when there is no dj_comment', () => {
    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.getAllByRole('row')).toHaveLength(2)
  })

  it('renders the matched/unmatched legend', () => {
    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.getByText('linked to artist page')).toBeInTheDocument()
    expect(screen.getByText('not matched yet')).toBeInTheDocument()
  })

  describe('live regime (PSY-1511)', () => {
    it('renders rows newest-first', () => {
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({ id: 1, position: 1, artist_name: 'CAN' }),
            makePlay({ id: 2, position: 2, artist_name: 'Neu!' }),
            makePlay({ id: 3, position: 3, artist_name: 'Harmonia' }),
          ]}
        />
      )
      const rows = screen.getAllByRole('row').slice(1)
      expect(rows[0]).toHaveTextContent('Harmonia')
      expect(rows[1]).toHaveTextContent('Neu!')
      expect(rows[2]).toHaveTextContent('CAN')
    })

    it('marks the newest row "▸ now" with the primary tint and gives older rows relative times', () => {
      const now = Date.now()
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({
              id: 1,
              position: 1,
              artist_name: 'CAN',
              air_timestamp: new Date(now - 9 * 60_000).toISOString(),
            }),
            makePlay({
              id: 2,
              position: 2,
              artist_name: 'Neu!',
              air_timestamp: new Date(now - 60_000).toISOString(),
            }),
          ]}
        />
      )
      const rows = screen.getAllByRole('row').slice(1)
      expect(rows[0]).toHaveTextContent('▸ now')
      expect(rows[0]).toHaveTextContent('Neu!')
      expect(rows[0]).toHaveAttribute('data-live-newest')
      expect(rows[1]).toHaveTextContent('9m')
      expect(rows[1]).not.toHaveAttribute('data-live-newest')
    })

    it('drops the "▸ now" marker for a stale newest row (honest relative time)', () => {
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({
              id: 1,
              position: 1,
              air_timestamp: new Date(Date.now() - 35 * 60_000).toISOString(),
            }),
          ]}
        />
      )
      const timeCell = document.querySelector('tbody td:first-child')
      expect(timeCell?.textContent).toBe('35m')
      // Still marked/tinted as the newest row — only the "now" claim goes.
      expect(screen.getAllByRole('row')[1]).toHaveAttribute('data-live-newest')
    })

    it('leaves older rows blank when the feed carried no timestamp', () => {
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({ id: 1, position: 1, air_timestamp: null }),
            makePlay({ id: 2, position: 2, artist_name: 'Neu!', air_timestamp: null }),
          ]}
        />
      )
      const timeCells = document.querySelectorAll('tbody td:first-child')
      expect(timeCells[0].textContent).toBe('▸ now')
      expect(timeCells[1].textContent).toBe('')
    })

    it('keeps the match affordances: dots, links, and suggest-a-match', () => {
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({ id: 1, position: 1, artist_id: 5, artist_slug: 'can' }),
            makePlay({ id: 2, position: 2, artist_name: 'The Tweeters' }),
          ]}
        />
      )
      expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
        'href',
        '/artists/can'
      )
      expect(screen.getByTestId('suggest-match-cta')).toBeInTheDocument()
    })

    it('keeps a dj_comment sub-row under its (reordered) track', () => {
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({ id: 1, position: 1, dj_comment: 'recorded in Forst' }),
            makePlay({ id: 2, position: 2, artist_name: 'Neu!' }),
          ]}
        />
      )
      const comment = screen.getByText('recorded in Forst')
      // CAN's row is now LAST; its comment row must directly follow it.
      const rows = screen.getAllByRole('row').slice(1)
      expect(rows[1]).toHaveTextContent('Mother Sky')
      expect(rows[2]).toBe(comment.closest('tr'))
    })

    it("extends the newest row's tint to its dj_comment sub-row", () => {
      render(
        <PlaylistTable
          live
          plays={[
            makePlay({ id: 1, position: 1 }),
            makePlay({
              id: 2,
              position: 2,
              artist_name: 'Neu!',
              dj_comment: 'live in the studio',
            }),
          ]}
        />
      )
      const commentRow = screen.getByText('live in the studio').closest('tr')
      expect(commentRow?.className).toContain('bg-primary/5')
    })
  })

  it('shows suggest-a-match CTA on unmatched rows for guests', () => {
    render(<PlaylistTable plays={[makePlay({ artist_name: 'The Tweeters' })]} />)
    expect(screen.getByTestId('suggest-match-cta')).toBeInTheDocument()
  })

  it('does not show suggest-a-match on matched rows', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ artist_id: 5, artist_slug: 'can' })]}
      />
    )
    expect(screen.queryByTestId('suggest-match-cta')).not.toBeInTheDocument()
    expect(screen.queryByTestId('suggest-match-pending')).not.toBeInTheDocument()
  })

  it('opens login prompt when a guest clicks suggest-a-match', async () => {
    const user = userEvent.setup()
    render(<PlaylistTable plays={[makePlay()]} />)
    await user.click(screen.getByTestId('suggest-match-cta'))
    expect(screen.getByTestId('login-prompt')).toBeInTheDocument()
  })

  it('shows suggestion pending when the caller has a pending suggestion', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockOwnPending.mockReturnValue({
      data: {
        id: 10,
        play_id: 1,
        status: 'pending',
        suggested_artist_name: 'CAN',
      },
      isLoading: false,
    })

    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.getByTestId('suggest-match-pending')).toHaveTextContent(
      'suggestion pending'
    )
    expect(screen.queryByTestId('suggest-match-cta')).not.toBeInTheDocument()
  })

  it('does not flash suggestion pending while the mine query is loading', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockOwnPending.mockReturnValue({
      data: undefined,
      isLoading: true,
    })

    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.queryByTestId('suggest-match-pending')).not.toBeInTheDocument()
    expect(screen.getByTestId('suggest-match-loading')).toBeInTheDocument()
    expect(screen.queryByTestId('suggest-match-cta')).not.toBeInTheDocument()
  })

  it('opens the picker for an authenticated user without a pending suggestion', async () => {
    mockAuthContext.mockReturnValue({
      user: { id: '1' },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    mockOwnPending.mockReturnValue({ data: null, isLoading: false })

    const user = userEvent.setup()
    render(<PlaylistTable plays={[makePlay({ artist_name: 'The Tweeters' })]} />)
    await user.click(screen.getByTestId('suggest-match-cta'))
    expect(screen.getByText('Suggest a match')).toBeInTheDocument()
    expect(screen.getByTestId('mock-artist-search')).toBeInTheDocument()
  })
})
