import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

const mockCreateToken = vi.fn()
const mockDeleteToken = vi.fn()
let mockHasToken = false
let mockCreateError: Error | null = null

vi.mock('@/features/auth', () => ({
  useCalendarTokenStatus: () => ({
    data: { has_token: mockHasToken },
    isLoading: false,
  }),
  useCreateCalendarToken: () => ({
    mutateAsync: mockCreateToken,
    isPending: false,
    isError: mockCreateError !== null,
    error: mockCreateError,
  }),
  useDeleteCalendarToken: () => ({
    mutateAsync: mockDeleteToken,
    isPending: false,
  }),
}))

import { FollowsActivityFeedSection } from './FollowsActivityFeedSection'
import {
  AuthError,
  AuthErrorCode,
  REAUTH_REQUIRED_MESSAGE,
} from '@/lib/errors'

describe('FollowsActivityFeedSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockHasToken = false
    mockCreateError = null
    mockCreateToken.mockResolvedValue({
      token: 'phcal_test',
      feed_url: 'https://api.example.com/feeds/phcal_test/saved-shows.ics',
      follows_feed_url:
        'https://api.example.com/feeds/phcal_test/follows.atom',
    })
  })

  // This card rotates the same personal feed token as the calendar card, so the
  // refusal has to reach it too. Without this the Enable button is a dead click.
  it('renders the sign-in-again copy when the feed mint refuses a stale session', () => {
    mockCreateError = new AuthError('refused', AuthErrorCode.REAUTH_REQUIRED, {
      status: 403,
    })
    renderWithProviders(<FollowsActivityFeedSection />)

    expect(screen.getByRole('alert').textContent).toContain(
      REAUTH_REQUIRED_MESSAGE
    )
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      expect.stringContaining('reason=REAUTH_REQUIRED')
    )
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

  // The "copied ✓" blip used to reset via an untracked `setTimeout`, so it
  // still fired ~2s after the section unmounted and called `setState` into a
  // torn-down React DOM. Under vitest that lands after jsdom teardown and
  // throws `ReferenceError: window is not defined`, failing the whole run with
  // every test passing. No timer may outlive the component.
  it('leaves no pending copied timer behind on unmount', async () => {
    vi.useFakeTimers()
    try {
      const writeText = vi.fn().mockResolvedValue(undefined)
      Object.assign(navigator, { clipboard: { writeText } })

      const { unmount } = renderWithProviders(<FollowsActivityFeedSection />)

      // Enabling reveals the plaintext URL (the only state with a copy button).
      // Both handlers are async, so drain microtasks after each click.
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Enable' }))
      })
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Copy feed URL' }))
      })
      expect(vi.getTimerCount()).toBeGreaterThan(0)

      unmount()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.useRealTimers()
    }
  })
})
