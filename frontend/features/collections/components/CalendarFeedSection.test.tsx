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

import { CalendarFeedSection } from './CalendarFeedSection'

describe('CalendarFeedSection', () => {
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

  it('renders the compact library setup card and keeps Enable functional', async () => {
    const { container } = renderWithProviders(
      <CalendarFeedSection variant="library" />
    )

    expect(screen.getByText('Subscribe to calendar')).toBeTruthy()
    expect(
      screen.getByText(
        'Sync your saved shows to Google Calendar, Apple Calendar, or Outlook.'
      )
    ).toBeTruthy()

    const card = container.firstElementChild as HTMLElement
    expect(card.className).toContain('rounded-md')
    expect(card.className).toContain('border-border')
    expect(card.className).not.toContain('border-dashed')

    fireEvent.click(screen.getByRole('button', { name: 'Enable' }))

    await waitFor(() => expect(mockCreateToken).toHaveBeenCalledTimes(1))
    expect(screen.getByText('Calendar Feed Active')).toBeTruthy()
    expect(screen.getByDisplayValue(/feeds\/phcal_test\/saved-shows\.ics/)).toBeTruthy()
    expect(screen.getByText('Google Calendar')).toBeTruthy()
    expect(screen.getByText(/Regenerate or disable from/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Regenerate' })).toBeNull()
  })

  it('library active-token state links to Settings instead of regenerating', () => {
    mockHasToken = true
    renderWithProviders(<CalendarFeedSection variant="library" />)

    expect(screen.getByText('Calendar feed enabled')).toBeTruthy()
    const manage = screen.getByRole('link', { name: 'Manage feed' })
    expect(manage.getAttribute('href')).toBe('/profile?tab=settings')
    expect(screen.queryByRole('button', { name: 'Regenerate' })).toBeNull()
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
})
