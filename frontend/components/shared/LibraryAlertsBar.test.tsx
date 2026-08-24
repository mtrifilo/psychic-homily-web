import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { LibraryAlertsBar } from './LibraryAlertsBar'

// The Library's alerts context bar (PSY-1905).

let mockPreferences: { home_metro: string | null } | undefined
let mockIsSuccess = true
const mockUseAlertPreferences = vi.fn()

vi.mock('@/features/auth/hooks/useAlertPreferences', () => ({
  useAlertPreferences: (enabled?: boolean) => {
    mockUseAlertPreferences(enabled)
    return { data: mockPreferences, isSuccess: mockIsSuccess }
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
    mockUseAlertPreferences.mockReset()
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

  it('always offers the custom alerts link and the pending-delivery note', () => {
    renderWithProviders(<LibraryAlertsBar entityType="venues" />)

    expect(
      screen.getByRole('link', { name: 'custom alerts →' })
    ).toHaveAttribute('href', '/settings/notification-filters')
    expect(screen.getByText(/still being switched on/i)).toBeInTheDocument()
  })
})
