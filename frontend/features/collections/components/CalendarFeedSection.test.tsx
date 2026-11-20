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

import { CalendarFeedSection } from './CalendarFeedSection'

describe('CalendarFeedSection', () => {
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

  // Without this the Enable button is a dead click: the mutation fails and the
  // card renders exactly what it rendered before. The mint is not gated on a
  // recent sign-in, so a failure (a rate-limit refusal, a server error) is one
  // to try again, with no sign-in link.
  it('renders the try-again copy when the feed mint fails', () => {
    mockCreateError = new Error('Too many requests')
    renderWithProviders(<CalendarFeedSection variant="settings" />)

    expect(screen.getByRole('alert').textContent).toContain(
      'Could not update the feed. Please try again.'
    )
    expect(screen.queryByRole('link', { name: 'Sign in again' })).toBeNull()
  })

  it('settings variant owns regenerate and shows leakage warning', async () => {
    renderWithProviders(<CalendarFeedSection variant="settings" />)

    expect(screen.getByText('Saved shows calendar feed')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Enable' }))

    await waitFor(() => expect(mockCreateToken).toHaveBeenCalledTimes(1))
    expect(
      screen.getByText(/Anyone with this URL can see your saved shows/)
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Disable' })).toBeTruthy()
  })

  it('settings active-token state exposes regenerate without URL', () => {
    mockHasToken = true
    const { container } = renderWithProviders(
      <CalendarFeedSection variant="settings" />
    )

    expect(screen.getByText('Saved shows calendar feed')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Disable' })).toBeTruthy()
    const layout = container.firstElementChild?.firstElementChild as HTMLElement
    expect(layout.className).toContain('flex-col')
    expect(layout.className).toContain('sm:flex-row')
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

      const { unmount } = renderWithProviders(
        <CalendarFeedSection variant="settings" />
      )

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
