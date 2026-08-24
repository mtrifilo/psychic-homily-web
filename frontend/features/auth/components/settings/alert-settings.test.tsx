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

let mockProfileLoaded = true
let mockProfileFailed = false

vi.mock('../../hooks/useAuth', () => ({
  useProfile: () => ({
    data: mockProfileData,
    isSuccess: mockProfileLoaded,
    isError: mockProfileFailed,
  }),
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
    mockProfileLoaded = true
    mockProfileFailed = false
  })

  // The reminder and digest rows read the PROFILE, not the alerts endpoint,
  // and they are the only in-product way to turn OFF a digest someone is
  // already receiving. `?? false` over an unresolved profile therefore told a
  // reader who came from an alert email "we are not emailing you" about three
  // streams we are, over a live checkbox that pins the wrong value on a click.
  describe('when the profile read has not resolved', () => {
    beforeEach(() => {
      mockProfileData = {}
      mockProfileLoaded = false
    })

    it('draws no checkbox for the profile-backed rows while pending', () => {
      renderWithProviders(<AlertSettings />)

      expect(
        screen.queryByRole('checkbox', { name: `Email: ${REMINDER_ROW}` })
      ).toBeNull()
      expect(
        screen.getAllByText(new RegExp(`Email: ${SCENE_ROW} is still loading`, 'i'))
      ).not.toHaveLength(0)
    })

    it('reports a failed profile read rather than showing every digest off', () => {
      mockProfileFailed = true
      renderWithProviders(<AlertSettings />)

      expect(
        screen.queryByRole('checkbox', { name: `Email: ${COLLECTION_ROW}` })
      ).toBeNull()
      expect(
        screen.getAllByText(new RegExp(`Email: ${COLLECTION_ROW} could not be loaded`, 'i'))
      ).not.toHaveLength(0)
    })
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

      // Two alerts now, not one: the matrix and the area card each read this
      // endpoint and each has to own up separately, because the area card's
      // silent version showed "No home area" to someone who has one.
      expect(
        screen.getAllByRole('alert').map(node => node.textContent).join(' ')
      ).toMatch(/Couldn't load your follow-alert settings/)
    })

    it('does not assert a home area it could not read', () => {
      renderWithProviders(<AlertSettings />)

      expect(screen.queryByTestId('home-metro-select')).toBeNull()
      expect(
        screen.getAllByRole('alert').map(node => node.textContent).join(' ')
      ).toMatch(/Couldn't load your area/)
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

  it('offers no account-level checkbox for custom alerts', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.queryByRole('checkbox', { name: /Custom alerts you built/ })
    ).not.toBeInTheDocument()
    // One "per filter", not two: only EMAIL is per filter. In-app fires
    // regardless, so it reads "always".
    expect(screen.getAllByText('per filter').length).toBe(1)
    expect(screen.getByText('always')).toBeInTheDocument()
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
  ])('parks the %s box while its own write is in flight', (_id, row, setPending) => {
    setPending()
    renderWithProviders(<AlertSettings />)
    // aria-disabled, NOT the disabled attribute: a disabled element cannot
    // hold focus, so parking a box during its own PATCH would drop a keyboard
    // user to <body> and restart their next Tab from the top of the page.
    // Same contract AlertChipRadioGroup states for the chips.
    const box = screen.getByRole('checkbox', { name: `Email: ${row}` })
    expect(box).toHaveAttribute('aria-disabled', 'true')
    expect(box).not.toBeDisabled()
  })

  it('keeps a parked box focusable so the keyboard user is not ejected', async () => {
    mockSceneDigestState = { isPending: true, isError: false }
    renderWithProviders(<AlertSettings />)

    const box = screen.getByRole('checkbox', { name: `Email: ${SCENE_ROW}` })
    box.focus()
    expect(box).toHaveFocus()
  })

  it('ignores a second write while one is already in flight', async () => {
    const user = userEvent.setup()
    mockSceneDigestState = { isPending: true, isError: false }
    renderWithProviders(<AlertSettings />)

    await user.click(screen.getByRole('checkbox', { name: `Email: ${SCENE_ROW}` }))
    expect(mockSetSceneDigest).not.toHaveBeenCalled()
  })

  // Each of these rows owns its own mutation, so one saving must not park the
  // controls of a row it has nothing to do with.
  it('leaves unrelated rows interactive while one row saves', () => {
    mockSceneDigestState = { isPending: true, isError: false }
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByRole('checkbox', { name: `Email: ${SCENE_ROW}` })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(
      screen.getByRole('checkbox', { name: `Email: ${COLLECTION_ROW}` })
    ).not.toHaveAttribute('aria-disabled')
    expect(
      screen.getByRole('checkbox', { name: `Email: ${REMINDER_ROW}` })
    ).not.toHaveAttribute('aria-disabled')
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

  // PENDING is not UNAVAILABLE. The first paint of the settings tab has no
  // matrix yet, and announcing "could not be loaded" over four settings whose
  // request is still in flight is a failure claim invented from a loading
  // state, with no banner to explain it because nothing failed.
  describe('while the preferences read is in flight', () => {
    beforeEach(() => {
      mockPreferences = undefined
      mockPreferencesLoading = true
      mockPreferencesFailed = false
    })

    it('says loading, not that the settings could not be loaded', () => {
      renderWithProviders(<AlertSettings />)

      expect(
        screen.getAllByText(/In-app: An artist or venue you follow announces a show is still loading/i)
      ).not.toHaveLength(0)
      expect(screen.queryByText(/could not be loaded/i)).toBeNull()
      expect(screen.queryByText(/^unknown$/i)).toBeNull()
    })

    it('claims no failure anywhere on the card', () => {
      renderWithProviders(<AlertSettings />)

      expect(screen.queryByRole('alert')).toBeNull()
    })

    // The area card obeys the same rule: "no home area" is a fact it does not
    // have yet, and asserting it over an ENABLED select invites a click that
    // overwrites a real stored metro.
    it('does not assert the viewer has no home area', () => {
      renderWithProviders(<AlertSettings />)

      expect(screen.queryByTestId('home-metro-select')).toBeNull()
      expect(screen.getByText(/Loading your area/i)).toBeInTheDocument()
    })
  })

  // Custom alerts always reach the inbox: the matcher writes the notification
  // row unconditionally and branches only on the email flag, and the builder's
  // own in-app switch is labelled "coming soon". "Per filter" would send
  // someone to a control that cannot turn this off.
  it('does not claim the custom-alert in-app channel is per filter', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(/In-app: Custom alerts you built is always on/i)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Email: Custom alerts you built is set on each custom alert/i)
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

  // Every sender this card governs takes an unsubscribeURL (show reminder,
  // filter match, both weekly digests, artist show alert), so the footnote can
  // and should cover all of them. Naming only two was precise and incomplete
  // at once, and the two weekly digests it omitted were MOVED into this card
  // by the same change.
  // The footnote has to survive two opposite failures: under-claiming (it
  // once named only 2 of the 5 unsubscribe-bearing rows) and over-claiming
  // (saying every unsubscribe "flips the same box you see here", which is
  // false for the custom-alerts row, whose link pauses one filter and whose
  // cell is deliberately not a box).
  it('scopes the unsubscribe promise to what this card governs', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(
        /Every email governed by this card stays off until you switch it on, row by row, and carries a one-click unsubscribe link/i
      )
    ).toBeInTheDocument()
  })

  it('names the custom-alert exception rather than claiming one rule for all', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(
        /except for a custom alert, which switches off email for that one alert instead/i
      )
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
