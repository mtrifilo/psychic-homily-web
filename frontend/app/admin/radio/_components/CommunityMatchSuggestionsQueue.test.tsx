import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CommunityMatchSuggestionsQueue } from './CommunityMatchSuggestionsQueue'
import type { RadioPlayMatchSuggestionEntry } from '@/lib/hooks/admin/useAdminRadio'

const mockList = vi.fn()
const mockAcceptMutate = vi.fn()
const mockRejectMutate = vi.fn()

vi.mock('@/lib/hooks/admin/useAdminRadio', () => ({
  useAdminMatchSuggestions: (...args: unknown[]) => mockList(...args),
  useAcceptMatchSuggestion: () => ({
    mutate: mockAcceptMutate,
    isPending: false,
  }),
  useRejectMatchSuggestion: () => ({
    mutate: mockRejectMutate,
    isPending: false,
  }),
}))

function makeSuggestion(
  overrides: Partial<RadioPlayMatchSuggestionEntry> = {}
): RadioPlayMatchSuggestionEntry {
  return {
    id: 7,
    play_id: 100,
    play_artist_name: 'The Tweeters',
    play_match_state: 'unmatched',
    suggested_artist_id: 42,
    suggested_artist_name: 'CAN',
    suggested_artist_slug: 'can',
    submitted_by: 3,
    submitter_username: 'matt',
    note: 'same band, different spelling',
    status: 'pending',
    created_at: '2026-07-18T12:00:00Z',
    ...overrides,
  }
}

describe('CommunityMatchSuggestionsQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders empty state when there are no pending suggestions', () => {
    mockList.mockReturnValue({
      data: { suggestions: [], total: 0 },
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
    })

    render(<CommunityMatchSuggestionsQueue />)
    expect(screen.getByTestId('community-match-suggestions-queue')).toBeInTheDocument()
    expect(
      screen.getByText('No pending community match suggestions.')
    ).toBeInTheDocument()
  })

  it('lists pending suggestions with accept/reject controls and bulk-link option', async () => {
    mockList.mockReturnValue({
      data: { suggestions: [makeSuggestion()], total: 1 },
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
    })

    const user = userEvent.setup()
    render(<CommunityMatchSuggestionsQueue />)

    expect(screen.getByText('Community suggestions')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByTestId('community-match-suggestion-row')).toBeInTheDocument()
    expect(screen.getByText('The Tweeters')).toBeInTheDocument()
    expect(screen.getByText('CAN')).toBeInTheDocument()
    expect(screen.getByText(/same band, different spelling/)).toBeInTheDocument()
    expect(screen.getByText('Also bulk-link this artist name')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Accept/i }))
    expect(mockAcceptMutate).toHaveBeenCalledWith(
      { suggestionId: 7, alsoBulkLinkName: false },
      expect.any(Object)
    )

    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: /Accept/i }))
    expect(mockAcceptMutate).toHaveBeenLastCalledWith(
      { suggestionId: 7, alsoBulkLinkName: true },
      expect.any(Object)
    )
  })

  it('rejects with a required reason', async () => {
    mockList.mockReturnValue({
      data: { suggestions: [makeSuggestion()], total: 1 },
      isLoading: false,
      isFetching: false,
      isError: false,
      error: null,
    })

    const user = userEvent.setup()
    render(<CommunityMatchSuggestionsQueue />)

    await user.click(screen.getByRole('button', { name: /^Reject$/i }))
    await user.type(
      screen.getByPlaceholderText(/Rejection reason/),
      'Wrong artist'
    )
    await user.click(screen.getByRole('button', { name: /Confirm Reject/i }))

    expect(mockRejectMutate).toHaveBeenCalledWith(
      { suggestionId: 7, reason: 'Wrong artist' },
      expect.any(Object)
    )
  })

  it('surfaces a load error', () => {
    mockList.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new Error('boom'),
    })

    render(<CommunityMatchSuggestionsQueue />)
    expect(
      screen.getByTestId('community-match-suggestions-load-error')
    ).toHaveTextContent('boom')
  })
})
