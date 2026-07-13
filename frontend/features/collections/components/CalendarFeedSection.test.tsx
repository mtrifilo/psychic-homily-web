import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

const mockCreateToken = vi.fn()
const mockDeleteToken = vi.fn()

vi.mock('@/features/auth', () => ({
  useCalendarTokenStatus: () => ({
    data: { has_token: false },
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
})
