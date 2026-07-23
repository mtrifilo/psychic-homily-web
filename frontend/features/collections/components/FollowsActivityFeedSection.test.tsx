import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

const mockCreateToken = vi.fn()
const mockDeleteToken = vi.fn()
let mockHasToken = false

vi.mock('@/features/auth', () => ({
  useCalendarTokenStatus: () => ({
    data: { has_token: mockHasToken },
    isLoading: false,
  }),
  useCreateCalendarToken: () => ({
    mutateAsync: mockCreateToken,
    isPending: false,
  }),
  useDeleteCalendarToken: () => ({
    mutateAsync: mockDeleteToken,
    isPending: false,
  }),
}))

import { FollowsActivityFeedSection } from './FollowsActivityFeedSection'

describe('FollowsActivityFeedSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasToken = false
    mockCreateToken.mockResolvedValue({
      token: 'phcal_test',
      feed_url: 'https://api.example.com/feeds/phcal_test/saved-shows.ics',
      follows_feed_url:
        'https://api.example.com/feeds/phcal_test/follows.atom',
    })
  })

  it('enables and shows the Atom URL with leakage copy', async () => {
    renderWithProviders(<FollowsActivityFeedSection />)

    expect(screen.getByText('Followed artists activity feed')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Enable' }))

    await waitFor(() => expect(mockCreateToken).toHaveBeenCalledTimes(1))
    expect(
      screen.getByDisplayValue(/feeds\/phcal_test\/follows\.atom/)
    ).toBeTruthy()
    expect(
      screen.getByText(/Anyone with this URL can see followed-artist activity/)
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Disable' })).toBeTruthy()
  })

  it('active-token state exposes regenerate without URL', () => {
    mockHasToken = true
    renderWithProviders(<FollowsActivityFeedSection />)

    expect(screen.getByText('Followed artists activity feed')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Disable' })).toBeTruthy()
    expect(screen.queryByLabelText('Follows activity feed URL')).toBeNull()
  })
})
