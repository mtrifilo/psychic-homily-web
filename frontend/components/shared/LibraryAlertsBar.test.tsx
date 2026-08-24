import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { LibraryAlertsBar } from './LibraryAlertsBar'

// The Library's alerts context bar (PSY-1905).

interface MockChannels {
  in_app: boolean
  email: boolean
}

let mockPreferences:
  | {
      home_metro: string | null
      alert_defaults?: { shows: MockChannels; releases: MockChannels }
    }
  | undefined
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
    mockPreferences = {
      home_metro: '38060',
      alert_defaults: {
        shows: { in_app: true, email: false },
        releases: { in_app: true, email: false },
      },
    }
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

  // The venues tab has no scope axis and so no area half, but `shows` is ONE
  // account key covering both follow types, so its follows pause on the same
  // fact and this tab has to be able to say so. The query is shared with the
  // entity pages and the settings card, so reading it here is a cache hit.
  it('reads the account matrix on a venues tab too, which pauses the same way', () => {
    renderWithProviders(<LibraryAlertsBar entityType="venues" />)
    expect(mockUseAlertPreferences).not.toHaveBeenCalledWith(false)
  })

  it('still omits the area half on a venues tab, which has no scope axis', () => {
    renderWithProviders(<LibraryAlertsBar entityType="venues" />)

    expect(screen.queryByText(/Your area/)).toBeNull()
    expect(screen.queryByText(/New follows start at/)).toBeNull()
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

  // A new follow inherits the ACCOUNT channels, so that is what decides this
  // half of the bar. With neither on, "New follows start at: Near me" reads
  // as a delivery promise directly above a column of brackets saying paused.
  describe('when the account matrix leaves no channel on', () => {
    const noChannels = () => {
      mockPreferences = {
        home_metro: '38060',
        alert_defaults: {
          shows: { in_app: false, email: false },
          releases: { in_app: true, email: false },
        },
      }
    }

    it('reports the pause instead of the starting scope', () => {
      noChannels()
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.getByText('paused')).toBeInTheDocument()
      expect(screen.queryByText(/New follows start at/)).toBeNull()
    })

    it('offers the way out once, rather than on every row', () => {
      noChannels()
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(
        screen.getByRole('link', { name: /paused.*alert settings/i })
      ).toHaveAttribute('href', '/profile?tab=settings#alerts')
    })

    // The area is what "near me" will mean once a channel comes back, and
    // this bar is the only place on the page it can be changed.
    it('keeps the area half, which the pause does not make meaningless', () => {
      noChannels()
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.getByText('Phoenix-Mesa-Chandler, AZ')).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Change your area' })
      ).toBeInTheDocument()
    })

    // RELEASE channels are a separate row of the same matrix. Reading the
    // wrong one would pause a tab over a setting that governs records.
    it('reads the shows row, not whichever row happens to be off', () => {
      mockPreferences = {
        home_metro: '38060',
        alert_defaults: {
          shows: { in_app: true, email: false },
          releases: { in_app: false, email: false },
        },
      }
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.getByText(/New follows start at/)).toBeInTheDocument()
      expect(screen.queryByText('paused')).toBeNull()
    })

    // The Venues tab has no starting-scope sentence to replace, but its rows
    // pause on the same account key, so it reports the pause and offers the
    // same way out rather than leaving a column of paused brackets unexplained
    // while the Artists tab one click away explains them.
    it('reports the pause on a venues tab, which has no scope sentence to replace', () => {
      noChannels()
      renderWithProviders(<LibraryAlertsBar entityType="venues" />)

      expect(screen.getByText('paused')).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: /paused.*alert settings/i })
      ).toBeInTheDocument()
      expect(screen.queryByText(/New follows start at/)).toBeNull()
      expect(screen.getByText(/still being switched on/i)).toBeInTheDocument()
    })

    // UNKNOWN is not "no channel". A pending read must not paint a pause over
    // a subscription that is delivering.
    it('stays silent while the matrix is still unknown', () => {
      mockPreferences = undefined
      renderWithProviders(<LibraryAlertsBar entityType="artists" />)

      expect(screen.queryByText('paused')).toBeNull()
    })
  })
})
