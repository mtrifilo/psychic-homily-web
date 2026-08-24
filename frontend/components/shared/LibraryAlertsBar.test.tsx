import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { LibraryAlertsBar } from './LibraryAlertsBar'

// The Library's alerts context bar (PSY-1905).

let mockPreferences: { home_metro: string | null } | undefined
let mockIsSuccess = true
let mockIsError = false
const mockUseAlertPreferences = vi.fn()
const mockRefetch = vi.fn()

vi.mock('@/features/auth/hooks/useAlertPreferences', () => ({
  useAlertPreferences: (enabled?: boolean) => {
    mockUseAlertPreferences(enabled)
    return {
      data: mockPreferences,
      isSuccess: mockIsSuccess,
      isError: mockIsError,
      refetch: mockRefetch,
    }
  },
}))

vi.mock('./HomeMetroField', () => ({
  HomeMetroSelect: () => <div data-testid="home-metro-select" />,
  useHomeMetroLabel: (metro: string | null | undefined) =>
    metro ? 'Phoenix-Mesa-Chandler, AZ' : null,
}))

describe('LibraryAlertsBar', () => {
  beforeEach(() => {
    mockPreferences = { home_metro: '38060' }
    mockIsSuccess = true
    mockIsError = false
    mockUseAlertPreferences.mockReset()
    mockRefetch.mockReset()
  })

  it('states the starting scope and the current area on a scoped tab', () => {
    renderWithProviders(<LibraryAlertsBar entityType="artists" />)

    expect(screen.getByText(/New follows start at/)).toBeInTheDocument()
    expect(screen.getByText('Near me')).toBeInTheDocument()
    expect(screen.getByText('Phoenix-Mesa-Chandler, AZ')).toBeInTheDocument()
  })

  it('says Everywhere when the viewer has no home area', () => {
    mockPreferences = { home_metro: null }
    renderWithProviders(<LibraryAlertsBar entityType="artists" />)

    expect(screen.getByText('Everywhere')).toBeInTheDocument()
    expect(screen.getByText('not set yet')).toBeInTheDocument()
  })

  // A venue sits in one place. Telling a venues tab that its follows start
  // "Near me", and offering an area control over them, describes a
  // restriction those follows do not have, and contradicts the venue control
  // one page over that says exactly that.
  it('omits the scope and area copy on a tab whose follows have no scope', () => {
    renderWithProviders(<LibraryAlertsBar entityType="venues" />)

    expect(screen.queryByText(/New follows start at/)).toBeNull()
    expect(screen.queryByText(/Your area/)).toBeNull()
    expect(screen.queryByRole('button', { name: 'Change your area' })).toBeNull()
  })

  it('does not fetch account preferences it cannot use on a venues tab', () => {
    renderWithProviders(<LibraryAlertsBar entityType="venues" />)
    expect(mockUseAlertPreferences).toHaveBeenCalledWith(false)
  })

  // Fails closed: a pending or failed read omits the area half rather than
  // guessing at it.
  it('omits the area copy until the preferences read resolves', () => {
    mockPreferences = undefined
    mockIsSuccess = false
    renderWithProviders(<LibraryAlertsBar entityType="artists" />)

    expect(screen.queryByText(/New follows start at/)).toBeNull()
    expect(screen.queryByText(/Your area/)).toBeNull()
  })

  it('always offers the custom alerts link', () => {
    renderWithProviders(<LibraryAlertsBar entityType="venues" />)

    expect(
      screen.getByRole('link', { name: 'custom alerts →' })
    ).toHaveAttribute('href', '/settings/notification-filters')
  })

  // PSY-1896 made delivery a per-type fact rather than one shared state, and
  // this bar is one of the two surfaces that has to honour it. Both arms are
  // pinned: an unconditional note is exactly the regression to catch, and it
  // would pass a presence-only test.
  describe('pending-delivery disclosure', () => {
    it('discloses it on the venues tab, where nothing delivers yet', () => {
      renderWithProviders(<LibraryAlertsBar entityType="venues" />)

      expect(screen.getByText(/still being switched on/i)).toBeInTheDocument()
    })

    it('stays silent on the artists tab, whose alerts already deliver', () => {
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.queryByText(/still being switched on/i)).toBeNull()
    })
  })

  // FAILED is not PENDING. On a failed read the bar loses its area half AND
  // every row bracket disappears (an unknown home area makes each menu render
  // null), so without a message the tab reads as "these follows carry no
  // alerts at all".
  describe('when the preferences read fails', () => {
    beforeEach(() => {
      mockPreferences = undefined
      mockIsSuccess = false
      mockIsError = true
    })

    it('says so rather than degrading silently', () => {
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't load your alert settings"
      )
    })

    it('offers a retry that refetches', async () => {
      const user = userEvent.setup()
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      await user.click(screen.getByRole('button', { name: 'retry' }))
      expect(mockRefetch).toHaveBeenCalled()
    })

    // Pending is still silent: only a real failure gets a message.
    it('stays silent while the read is merely in flight', () => {
      mockIsError = false
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.queryByRole('alert')).toBeNull()
    })
  })
})
