import { describe, it, expect, vi, beforeEach } from 'vitest'
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

vi.mock('../../hooks/useAlertPreferences', () => ({
  useAlertPreferences: () => ({
    data: mockPreferences,
    isLoading: mockPreferencesLoading,
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
    mockProfileData = { user: { preferences: {} } }
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

  // Capability truth: delivery is a separate, unshipped piece of work, and
  // this card must not imply mail is already flowing from it.
  it('says plainly that follow-driven alerts are not switched on yet', () => {
    renderWithProviders(<AlertSettings />)

    expect(
      screen.getByText(/still being switched on/i)
    ).toBeInTheDocument()
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
