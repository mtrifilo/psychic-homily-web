import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { AlertSettings } from './alert-settings'
import type { AlertPreferences } from '../../hooks/useAlertPreferences'

// The account alert matrix (PSY-1905). Also carries the coverage that moved
// here with the show-reminder and weekly-digest rows.

const mockSetAlertDefaults = vi.fn()
let mockAlertDefaultsState = { isPending: false, isError: false }

const mockSetShowReminders = vi.fn()
let mockShowRemindersState = { isPending: false, isError: false }

const mockSetCollectionDigest = vi.fn()
let mockCollectionDigestState = { isPending: false, isError: false }

const mockSetSceneDigest = vi.fn()
let mockSceneDigestState = { isPending: false, isError: false }

let mockPreferences: AlertPreferences | undefined
let mockPreferencesLoading = false

let mockProfileData: {
  user?: {
    preferences?: {
      show_reminders?: boolean
      notify_on_collection_digest?: boolean
      notify_on_scene_digest?: boolean
    }
  }
} = {}

vi.mock('../../hooks/useAuth', () => ({
  useProfile: () => ({ data: mockProfileData }),
}))

let mockPreferencesFailed = false

vi.mock('../../hooks/useAlertPreferences', () => ({
  useAlertPreferences: () => ({
    data: mockPreferences,
    isLoading: mockPreferencesLoading,
    isError: mockPreferencesFailed,
  }),
  useSetAlertDefaults: () => ({
    mutate: mockSetAlertDefaults,
    ...mockAlertDefaultsState,
  }),
  useSetHomeMetro: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
}))

vi.mock('@/features/shows', () => ({
  useSetShowReminders: () => ({
    mutate: mockSetShowReminders,
    ...mockShowRemindersState,
  }),
}))

vi.mock('@/features/collections', () => ({
  useSetCollectionDigestPreference: () => ({
    mutate: mockSetCollectionDigest,
    ...mockCollectionDigestState,
  }),
}))

vi.mock('@/features/scenes', () => ({
  useSetSceneDigestPreference: () => ({
    mutate: mockSetSceneDigest,
    ...mockSceneDigestState,
  }),
}))

// The metro picker owns a query of its own; the "Your area" card's job here is
// only to exist and be anchored.
vi.mock('@/components/shared/HomeMetroField', () => ({
  HomeMetroSelect: ({ metro }: { metro: string | null }) => (
    <div data-testid="home-metro-select" data-metro={metro ?? ''} />
  ),
  useHomeMetroLabel: () => null,
}))

const preferences = (
  overrides: Partial<AlertPreferences> = {}
): AlertPreferences => ({
  success: true,
  home_metro: '38060',
  alert_defaults: {
    shows: { in_app: true, email: false },
    releases: { in_app: true, email: false },
  },
  ...overrides,
})

const SHOWS_ROW = 'An artist or venue you follow announces a show'
const RELEASES_ROW = 'An artist you follow puts out a release'
const REMINDER_ROW = 'Day-before reminder for a show you saved'
const SCENE_ROW = 'Weekly digest for scenes you follow'
const COLLECTION_ROW = 'Weekly digest for collections you follow'

describe('AlertSettings', () => {
  beforeEach(() => {
    mockSetAlertDefaults.mockReset()
    mockAlertDefaultsState = { isPending: false, isError: false }
    mockSetShowReminders.mockReset()
    mockShowRemindersState = { isPending: false, isError: false }
    mockSetCollectionDigest.mockReset()
    mockCollectionDigestState = { isPending: false, isError: false }
    mockSetSceneDigest.mockReset()
    mockSceneDigestState = { isPending: false, isError: false }
    mockPreferences = preferences()
    mockPreferencesLoading = false
    mockPreferencesFailed = false
    mockProfileData = { user: { preferences: {} } }
  })

  // A failed read used to fall straight through to the table, painting the
  // shipped defaults as the user's SAVED state: someone with show emails on
  // was told they were off, over live checkboxes that would then pin the
  // wrong value on one click.
  describe('when the preferences read fails', () => {
    beforeEach(() => {
      mockPreferences = undefined
      mockPreferencesFailed = true
    })

    it('says so instead of drawing invented settings', () => {
      renderWithProviders(<AlertSettings />)

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't load your follow-alert settings"
      )
    })

    it('offers no live control that could pin a value from unknown state', () => {
      renderWithProviders(<AlertSettings />)

      for (const row of [SHOWS_ROW, RELEASES_ROW]) {
        expect(
          screen.queryByRole('checkbox', { name: `In-app: ${row}` })
        ).toBeNull()
        expect(
          screen.queryByRole('checkbox', { name: `Email: ${row}` })
        ).toBeNull()
      }
    })

    // The failure is scoped to the two rows that read the account matrix. The
    // reminder and both digests read the profile, and they are the only
    // in-product way to turn OFF a digest someone is already receiving: a new
    // endpoint's bad day must not strand a shipped preference.
    it('keeps the profile-backed rows usable', () => {
      renderWithProviders(<AlertSettings />)

      for (const row of [REMINDER_ROW, SCENE_ROW, COLLECTION_ROW]) {
        expect(
          screen.getByRole('checkbox', { name: `Email: ${row}` })
        ).toBeEnabled()
      }
    })

    // Same reason: the area card reads `home_metro`, not the matrix, and the
    // "set your area" link from an entity page points straight at it.
    it('keeps the area card and its anchor', () => {
      const { container } = renderWithProviders(<AlertSettings />)

      expect(screen.getByText('Your area')).toBeInTheDocument()
      expect(container.querySelector('#alerts-area')).not.toBeNull()
    })
  })

  it('draws no follow-alert checkbox while the read is in flight', () => {
    mockPreferences = undefined
    mockPreferencesLoading = true
    renderWithProviders(<AlertSettings />)

    expect(
      screen.queryByRole('checkbox', { name: `Email: ${SHOWS_ROW}` })
    ).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('renders the card and both channel columns', () => {
    renderWithProviders(<AlertSettings />)

    expect(screen.getByText('Alerts')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'In-app' })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Email' })).toBeInTheDocument()
  })

  it('renders every alert row', () => {
    renderWithProviders(<AlertSettings />)

    for (const row of [
      SHOWS_ROW,
      RELEASES_ROW,
      REMINDER_ROW,
      SCENE_ROW,
      COLLECTION_ROW,
      'Custom alerts you built',
    ]) {
      expect(screen.getByText(row)).toBeInTheDocument()
    }
  })

  // The locked PSY-1892 default: in-app ON, email OFF, per alert type. The
  // asymmetry is the whole decision, so it is pinned rather than assumed.
  it('draws the decided defaults: in-app on, email off', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByRole('checkbox', { name: `In-app: ${SHOWS_ROW}` })
    ).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: `Email: ${SHOWS_ROW}` })
    ).not.toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: `In-app: ${RELEASES_ROW}` })
    ).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: `Email: ${RELEASES_ROW}` })
    ).not.toBeChecked()
  })

  it('reflects a stored matrix that differs from the shipped default', () => {
    mockPreferences = preferences({
      alert_defaults: {
        shows: { in_app: false, email: true },
        releases: { in_app: true, email: false },
      },
    })
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByRole('checkbox', { name: `In-app: ${SHOWS_ROW}` })
    ).not.toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: `Email: ${SHOWS_ROW}` })
    ).toBeChecked()
  })

  // A PATCH carries only the axis that changed; pinning the other one would
  // freeze today's shipped default onto the row.
  it('sends only the flipped channel for the shows row', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AlertSettings />)

    await user.click(
      screen.getByRole('checkbox', { name: `Email: ${SHOWS_ROW}` })
    )

    expect(mockSetAlertDefaults).toHaveBeenCalledTimes(1)
    expect(mockSetAlertDefaults).toHaveBeenCalledWith({
      shows: { email: true },
    })
  })

  it('sends only the flipped channel for the releases row', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AlertSettings />)

    await user.click(
      screen.getByRole('checkbox', { name: `In-app: ${RELEASES_ROW}` })
    )

    expect(mockSetAlertDefaults).toHaveBeenCalledWith({
      releases: { in_app: false },
    })
  })

  // ----- Rows that moved here from the Notifications card -----

  it('drives the day-before reminder through its own preference', async () => {
    mockProfileData = { user: { preferences: { show_reminders: false } } }
    const user = userEvent.setup()
    renderWithProviders(<AlertSettings />)

    const box = screen.getByRole('checkbox', { name: `Email: ${REMINDER_ROW}` })
    expect(box).not.toBeChecked()
    await user.click(box)
    expect(mockSetShowReminders).toHaveBeenCalledWith(true)
  })

  it('drives the scene digest through its own preference', async () => {
    mockProfileData = { user: { preferences: { notify_on_scene_digest: true } } }
    const user = userEvent.setup()
    renderWithProviders(<AlertSettings />)

    const box = screen.getByRole('checkbox', { name: `Email: ${SCENE_ROW}` })
    expect(box).toBeChecked()
    await user.click(box)
    expect(mockSetSceneDigest).toHaveBeenCalledWith(false)
  })

  it('drives the collection digest through its own preference', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AlertSettings />)

    await user.click(
      screen.getByRole('checkbox', { name: `Email: ${COLLECTION_ROW}` })
    )
    expect(mockSetCollectionDigest).toHaveBeenCalledWith(true)
  })

  it('defaults both digests OFF when the preference is absent, matching the server', () => {
    mockProfileData = { user: { preferences: {} } }
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByRole('checkbox', { name: `Email: ${SCENE_ROW}` })
    ).not.toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: `Email: ${COLLECTION_ROW}` })
    ).not.toBeChecked()
  })

  // A dash is not a checkbox: an email-only alert has no in-app switch to be
  // "off", and rendering a disabled box there would claim otherwise.
  it('renders no in-app checkbox for the email-only rows', () => {
    renderWithProviders(<AlertSettings />)

    for (const row of [REMINDER_ROW, SCENE_ROW, COLLECTION_ROW]) {
      expect(
        screen.queryByRole('checkbox', { name: `In-app: ${row}` })
      ).not.toBeInTheDocument()
    }
  })

  it('offers no account-level checkbox for custom alerts, whose channels are per filter', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.queryByRole('checkbox', { name: /Custom alerts you built/ })
    ).not.toBeInTheDocument()
    expect(screen.getAllByText('per filter').length).toBe(2)
  })

  it('surfaces a failed write', () => {
    mockAlertDefaultsState = { isPending: false, isError: true }
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByRole('alert')
    ).toHaveTextContent('Failed to update setting. Please try again.')
  })

  // Coverage that moved here with the rows, from notification-settings.test.
  it.each([
    ['show-reminders', REMINDER_ROW, () => (mockShowRemindersState = { isPending: true, isError: false })],
    ['scene-digest', SCENE_ROW, () => (mockSceneDigestState = { isPending: true, isError: false })],
    ['collection-digest', COLLECTION_ROW, () => (mockCollectionDigestState = { isPending: true, isError: false })],
  ])('disables the %s box while its own write is in flight', (_id, row, setPending) => {
    setPending()
    renderWithProviders(<AlertSettings />)
    expect(screen.getByRole('checkbox', { name: `Email: ${row}` })).toBeDisabled()
  })

  // Each of these rows owns its own mutation, so one saving must not park the
  // controls of a row it has nothing to do with.
  it('leaves unrelated rows interactive while one row saves', () => {
    mockSceneDigestState = { isPending: true, isError: false }
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByRole('checkbox', { name: `Email: ${SCENE_ROW}` })
    ).toBeDisabled()
    expect(
      screen.getByRole('checkbox', { name: `Email: ${COLLECTION_ROW}` })
    ).toBeEnabled()
    expect(
      screen.getByRole('checkbox', { name: `Email: ${REMINDER_ROW}` })
    ).toBeEnabled()
  })

  it.each([
    ['show reminder', () => (mockShowRemindersState = { isPending: false, isError: true })],
    ['scene digest', () => (mockSceneDigestState = { isPending: false, isError: true })],
    ['collection digest', () => (mockCollectionDigestState = { isPending: false, isError: true })],
  ])('surfaces a failed %s write', (_label, setError) => {
    setError()
    renderWithProviders(<AlertSettings />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Failed to update setting. Please try again.'
    )
  })

  // Capability truth, and it is now per alert type. Artist show alerts DELIVER
  // (PSY-1896); venue show alerts and release alerts do not. Claiming either
  // state for the wrong one is a lie in one direction or the other.
  it('discloses only the alert types that do not deliver yet', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(/Alerts for shows a venue you follow adds are still being switched on/i)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Release alerts are still being switched on/i)
    ).toBeInTheDocument()
  })

  // The claim is scoped to the channel that has been observed delivering.
  // PSY-1896's email lane is built and integration-tested but has never sent a
  // real message, so "live" may not stretch across the Email column.
  it('claims delivery only for the artist in-app channel', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(
        /In-app alerts for artists are live; venue alerts are still being switched on/i
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/^Artist alerts are live/i)).not.toBeInTheDocument()
  })

  // Only the artist show-alert email exists, so the unsubscribe footnote may
  // not generalize over show-alert emails that have no notifier.
  it('scopes the unsubscribe footnote to the emails that exist', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(/Artist show-alert and reminder emails carry a one-click unsubscribe/i)
    ).toBeInTheDocument()
  })

  // PSY-1896's artist show-alert email links its "manage" CTA here.
  it('anchors the matrix so an alert email can deep-link to it', () => {
    const { container } = renderWithProviders(<AlertSettings />)
    expect(container.querySelector('#alerts')).not.toBeNull()
  })

  // Both anchors sit inside a lazily-mounted TabsContent, so the browser has
  // already given up on the fragment by the time the card exists. An `id`
  // alone therefore proves nothing: the card has to scroll ITSELF into view
  // when the node arrives. The area card had that; the matrix did not, which
  // stranded PSY-1896's emailed reader two cards above what they were sent to.
  describe('cold-load deep links', () => {
    // jsdom does not implement scrollIntoView, so this is a stub rather than a
    // spy. Restored after each case so the rest of the suite is unaffected.
    const originalScrollIntoView = Element.prototype.scrollIntoView

    const renderAtHash = (hash: string) => {
      window.location.hash = hash
      const scrollIntoView = vi.fn()
      Element.prototype.scrollIntoView = scrollIntoView
      const { container } = renderWithProviders(<AlertSettings />)
      return { container, scrollIntoView }
    }

    afterEach(() => {
      window.location.hash = ''
      Element.prototype.scrollIntoView = originalScrollIntoView
    })

    it('scrolls the matrix into view for #alerts', () => {
      const { container, scrollIntoView } = renderAtHash('#alerts')

      expect(scrollIntoView).toHaveBeenCalled()
      expect(scrollIntoView.mock.instances[0]).toBe(
        container.querySelector('#alerts')
      )
    })

    it('scrolls the area card into view for #alerts-area', () => {
      const { container, scrollIntoView } = renderAtHash('#alerts-area')

      expect(scrollIntoView).toHaveBeenCalled()
      expect(scrollIntoView.mock.instances[0]).toBe(
        container.querySelector('#alerts-area')
      )
    })

    it('scrolls nothing when the settings tab is opened without a fragment', () => {
      const { scrollIntoView } = renderAtHash('')

      expect(scrollIntoView).not.toHaveBeenCalled()
    })
  })

  it('anchors the area card so "set your area" links can land on it', () => {
    const { container } = renderWithProviders(<AlertSettings />)

    expect(screen.getByText('Your area')).toBeInTheDocument()
    expect(container.querySelector('#alerts-area')).not.toBeNull()
    expect(screen.getByTestId('home-metro-select')).toHaveAttribute(
      'data-metro',
      '38060'
    )
  })
})
