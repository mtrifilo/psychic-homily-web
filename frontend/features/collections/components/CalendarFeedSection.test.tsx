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
      token: 'calendar-token',
      feed_url: 'https://example.com/calendar.ics',
    })
  })

  it('renders the compact setup card and keeps Enable functional', async () => {
    const { container } = renderWithProviders(<CalendarFeedSection />)

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
  })

  it('stacks active-token details and actions on narrow screens', () => {
    mockHasToken = true
    const { container } = renderWithProviders(<CalendarFeedSection />)

    expect(screen.getByText('Calendar Feed Active')).toBeTruthy()
    const layout = container.firstElementChild?.firstElementChild as HTMLElement
    expect(layout.className).toContain('flex-col')
    expect(layout.className).toContain('sm:flex-row')
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Disable' })).toBeTruthy()
  })
})
