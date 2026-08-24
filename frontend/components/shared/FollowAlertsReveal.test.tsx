import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { FollowAlertsReveal } from './FollowAlertsReveal'
import type { FollowAlertScope, FollowAlertSettings } from '@/lib/types/follow'

// The post-follow scope reveal of the merged Follow control (PSY-1905).

const mockUpdate = vi.fn()
let mockUpdateState = { isPending: false, isError: false }
let mockIsAuthenticated = true
let mockIsFollowing = true
let mockAlerts: FollowAlertSettings | undefined
let mockHomeMetro: string | null = '38060'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockIsAuthenticated,
    user: { id: 1 },
  }),
}))

vi.mock('@/lib/hooks/common/useFollow', () => ({
  useFollowStatus: () => ({ data: { is_following: mockIsFollowing } }),
  followMutationKey: ['follow-entity'],
}))

// Captures the `enabled` argument so the optimistic-follow race is assertable.
const followAlertsEnabled = vi.fn()
let mockAlertsFailed = false
let mockAlertsErrorStatus: number | undefined
const mockRefetchAlerts = vi.fn()

vi.mock('@/lib/hooks/common/useFollowAlerts', () => ({
  useFollowAlerts: (_type: string, _id: number, enabled?: boolean) => {
    followAlertsEnabled(enabled)
    return {
      data: enabled ? mockAlerts : undefined,
      isError: mockAlertsFailed,
      error: mockAlertsFailed
        ? { status: mockAlertsErrorStatus ?? 500 }
        : undefined,
      refetch: mockRefetchAlerts,
    }
  },
  useUpdateFollowAlerts: () => ({ mutate: mockUpdate, ...mockUpdateState }),
  // The real predicate, not a stub: the component and the retry policy have to
  // agree on what a 404 means, and a stub here would let them drift.
  isFollowAlertsNotFound: (error: unknown) =>
    (error as { status?: number })?.status === 404,
}))

let mockIsMutating = 0

vi.mock('@tanstack/react-query', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-query')>()
  return { ...actual, useIsMutating: () => mockIsMutating }
})

let mockPreferencesResolved = true
let mockPreferencesFailed = false
const mockRefetchPreferences = vi.fn()

vi.mock('@/features/auth/hooks/useAlertPreferences', () => ({
  useAlertPreferences: () => ({
    data: mockPreferencesResolved ? { home_metro: mockHomeMetro } : undefined,
    isSuccess: mockPreferencesResolved,
    isError: mockPreferencesFailed,
    refetch: mockRefetchPreferences,
  }),
  useHomeMetroState: () =>
    mockPreferencesResolved ? Boolean(mockHomeMetro) : undefined,
}))

const artistAlerts = (
  scope: FollowAlertScope | undefined = 'near_me',
  enabled = true
): FollowAlertSettings => ({
  entity_type: 'artist',
  entity_id: 7,
  shows: { enabled, in_app: true, email: false, scope },
})

const renderArtist = () =>
  renderWithProviders(
    <FollowAlertsReveal
      entityType="artists"
      entityId={7}
      entityName="Just Mustard"
    />
  )

const renderVenue = () =>
  renderWithProviders(
    <FollowAlertsReveal
      entityType="venues"
      entityId={4}
      entityName="Rebel Lounge"
    />
  )

describe('FollowAlertsReveal', () => {
  beforeEach(() => {
    mockUpdate.mockReset()
    mockUpdateState = { isPending: false, isError: false }
    mockIsAuthenticated = true
    mockIsFollowing = true
    mockAlerts = artistAlerts()
    mockHomeMetro = '38060'
    mockPreferencesResolved = true
    mockPreferencesFailed = false
    mockAlertsFailed = false
    mockAlertsErrorStatus = undefined
    mockIsMutating = 0
    followAlertsEnabled.mockReset()
    mockRefetchAlerts.mockReset()
    mockRefetchPreferences.mockReset()
  })

  // FAILED is not PENDING. Rendering null for both means one 4xx (which the
  // global retry policy does not retry at all) leaves a page reading
  // [Following] with no control, no message and no way back, so the user
  // reasonably concludes the follow did not subscribe.
  describe('when a read fails', () => {
    it('says so instead of vanishing when the subscription read fails', () => {
      mockAlerts = undefined
      mockAlertsFailed = true
      renderArtist()

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't load your alert settings for Just Mustard."
      )
    })

    it('says so when the account preferences read fails', () => {
      mockPreferencesResolved = false
      mockPreferencesFailed = true
      renderArtist()

      expect(screen.getByRole('alert')).toBeInTheDocument()
    })

    it('offers a retry that refetches both reads', async () => {
      const user = userEvent.setup()
      mockAlerts = undefined
      mockAlertsFailed = true
      renderArtist()

      await user.click(screen.getByRole('button', { name: 'retry' }))
      expect(mockRefetchAlerts).toHaveBeenCalled()
      expect(mockRefetchPreferences).toHaveBeenCalled()
    })

    // Still nothing while merely pending: an error message on every first
    // paint would be its own kind of lie.
    it('stays silent while the reads are merely pending', () => {
      mockAlerts = undefined
      const { container } = renderArtist()
      expect(container).toBeEmptyDOMElement()
    })
  })

  // Treating a pending read as "no home area" renders the two-chip set with
  // Everywhere selected for someone whose stored scope is near-me, offers them
  // a link to set an area they already have, and then swaps the chips out from
  // under them. A click landing in that window on the chip that is about to
  // become current is swallowed by the equal-value guard, so their correction
  // silently does nothing.
  it('renders nothing for a scoped follow until the home area is known', () => {
    mockPreferencesResolved = false
    const { container } = renderArtist()
    expect(container).toBeEmptyDOMElement()
  })

  it('never offers to set an area while the area is still unknown', () => {
    mockPreferencesResolved = false
    renderArtist()
    expect(screen.queryByRole('link', { name: 'set your area' })).toBeNull()
  })

  // A venue has no scope axis, so it must not be held up by a read it does
  // not use.
  it('renders a venue immediately, without waiting on the home area', () => {
    mockPreferencesResolved = false
    renderWithProviders(
      <FollowAlertsReveal
        entityType="venues"
        entityId={4}
        entityName="Rebel Lounge"
      />
    )
    expect(screen.getByRole('radio', { name: 'On' })).toBeChecked()
  })

  // The follow flips optimistically, so `is_following` reads true before the
  // POST lands. The alerts endpoint is a sub-resource of that follow and 404s
  // until it exists, so asking on the optimistic flag spends a
  // guaranteed-failing request and a logged error on the happy path.
  it('waits for the follow write to settle before asking for its alerts', () => {
    mockIsMutating = 1
    renderArtist()

    expect(followAlertsEnabled).toHaveBeenCalledWith(false)
  })

  it('asks for the alerts once the follow write has settled', () => {
    renderArtist()
    expect(followAlertsEnabled).toHaveBeenCalledWith(true)
  })

  // The first click stays one click: the scope question does not exist until
  // the subscription does.
  it('renders nothing before the entity is followed', () => {
    mockIsFollowing = false
    const { container } = renderArtist()
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing for an anonymous viewer', () => {
    mockIsAuthenticated = false
    const { container } = renderArtist()
    expect(container).toBeEmptyDOMElement()
  })

  // 422 territory: a label follow carries no subscription at all.
  it('renders nothing for a follow type with no alert subscription', () => {
    const { container } = renderWithProviders(
      <FollowAlertsReveal entityType="labels" entityId={3} entityName="Sub Pop" />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing until the resolved subscription lands', () => {
    mockAlerts = undefined
    const { container } = renderArtist()
    expect(container).toBeEmptyDOMElement()
  })

  it('marks the stored scope as the checked radio', () => {
    renderArtist()

    expect(screen.getByRole('radio', { name: 'Near me' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Everywhere' })).not.toBeChecked()
    expect(screen.getByRole('radio', { name: 'Off' })).not.toBeChecked()
  })

  it('names the group after the entity so several on a page stay distinct', () => {
    renderArtist()
    expect(
      screen.getByRole('radiogroup', { name: 'Alerts for Just Mustard' })
    ).toBeInTheDocument()
  })

  it('writes the chosen scope', async () => {
    const user = userEvent.setup()
    renderArtist()

    await user.click(screen.getByRole('radio', { name: 'Everywhere' }))

    expect(mockUpdate).toHaveBeenCalledWith({
      entityType: 'artists',
      entityId: 7,
      update: { shows: { enabled: true, scope: 'everywhere' } },
    })
  })

  it('does not re-write the choice already in effect', async () => {
    const user = userEvent.setup()
    renderArtist()

    await user.click(screen.getByRole('radio', { name: 'Near me' }))

    expect(mockUpdate).not.toHaveBeenCalled()
  })

  // Near me is withheld without an area, so the way to get one sits right
  // where it is missing.
  it('offers a way to set an area instead of a near-me option when none is set', () => {
    mockHomeMetro = null
    renderArtist()

    expect(screen.queryByRole('radio', { name: 'Near me' })).toBeNull()
    expect(screen.getByRole('link', { name: 'set your area' })).toHaveAttribute(
      'href',
      '/profile?tab=settings#alerts-area'
    )
  })

  it('drops the set-your-area link once an area exists', () => {
    renderArtist()
    expect(screen.queryByRole('link', { name: 'set your area' })).toBeNull()
  })

  it('gives a venue an on/off axis and no scope', () => {
    renderWithProviders(
      <FollowAlertsReveal
        entityType="venues"
        entityId={4}
        entityName="Rebel Lounge"
      />
    )

    expect(screen.getByRole('radio', { name: 'On' })).toBeChecked()
    expect(screen.getByRole('radio', { name: 'Off' })).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: 'Near me' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'set your area' })).toBeNull()
  })

  it('reads a disabled subscription as off', () => {
    mockAlerts = artistAlerts('near_me', false)
    renderArtist()
    expect(screen.getByRole('radio', { name: 'Off' })).toBeChecked()
  })

  it('surfaces a failed write rather than silently reverting', () => {
    mockUpdateState = { isPending: false, isError: true }
    renderArtist()
    expect(screen.getByRole('alert')).toHaveTextContent(
      "Couldn't save that. Try again."
    )
  })

  // Parked via aria-disabled, not the disabled attribute: a disabled button
  // cannot hold focus, so parking the group mid-write would eject a keyboard
  // user out of it entirely. See AlertChipRadioGroup.test.
  it('parks the chips while a write is in flight', () => {
    mockUpdateState = { isPending: true, isError: false }
    renderArtist()
    expect(screen.getByRole('radio', { name: 'Everywhere' })).toHaveAttribute(
      'aria-disabled',
      'true'
    )
  })

  // A 404 on the alerts sub-resource means "not following", which is the one
  // error the retry policy deliberately refuses to retry. Treating it as a
  // load failure paints a message that can never clear.
  describe('when the alerts read 404s', () => {
    beforeEach(() => {
      mockAlerts = undefined
      mockAlertsFailed = true
      mockAlertsErrorStatus = 404
    })

    // Real sequence: this tab's follow status is stale for its 2-minute
    // window, so `is_following` still reads true after another tab unfollows.
    it('renders nothing rather than an error it cannot retry away', () => {
      const { container } = renderArtist()

      expect(screen.queryByRole('alert')).toBeNull()
      expect(screen.queryByRole('button', { name: 'retry' })).toBeNull()
      expect(container).toBeEmptyDOMElement()
    })

    // Anything that is NOT a 404 still gets the message and the retry, which
    // is the behaviour this must not regress.
    it('still reports a genuine failure', () => {
      mockAlertsErrorStatus = 500
      renderArtist()

      expect(screen.getByRole('alert')).toHaveTextContent(
        "Couldn't load your alert settings"
      )
    })
  })

  // Capability truth, and it is PER TYPE since PSY-1896: artist show alerts
  // deliver, venue ones do not. Both tooltips have to be pinned to the right
  // side of that line, and pinned to the CLAIM rather than to the existence of
  // a tooltip button.
  describe('capability truth in the tooltips', () => {
    const openTooltip = async () => {
      const user = userEvent.setup()
      await user.hover(
        screen.getByRole('button', { name: 'What these alerts cover' })
      )
    }

    // The artist tooltip's only pending sentence is about RELEASES. A regex
    // for "still being switched on" alone passes even if the artist copy is
    // reverted to claiming artist SHOW alerts are unshipped, because the
    // release note satisfies it. So assert the release subject explicitly, and
    // assert the reverted claim is absent.
    it('scopes the artist tooltip pending note to releases only', async () => {
      renderArtist()
      await openTooltip()

      expect(
        await screen.findAllByText(/Release alerts are still being switched on/i)
      ).not.toHaveLength(0)
      expect(
        screen.queryByText(/Alerts from the artists and venues you follow/i)
      ).toBeNull()
      expect(
        screen.queryByText(/shows a venue you follow adds are still being switched on/i)
      ).toBeNull()
    })

    it('discloses pending delivery in the venue tooltip', async () => {
      renderVenue()
      await openTooltip()

      expect(
        await screen.findAllByText(
          /Alerts for shows a venue you follow adds are still being switched on/i
        )
      ).not.toHaveLength(0)
    })

    // The venue tooltip used to open with "Turns alerts on or off", a
    // present-tense capability claim, and then say two sentences later that
    // those same alerts are not on yet. One tooltip, contradicting itself.
    it('does not open the venue tooltip with a present-tense delivery claim', async () => {
      renderVenue()
      await openTooltip()

      await screen.findAllByText(/still being switched on/i)
      expect(screen.queryByText(/^Turns alerts on or off/i)).toBeNull()
    })

    // A venue has no geography, so it must never be told about scope.
    it('never mentions near me in the venue tooltip', async () => {
      renderVenue()
      await openTooltip()

      await screen.findAllByText(/still being switched on/i)
      expect(screen.queryByText(/near me/i)).toBeNull()
    })
  })

  // Enabled, and reaching nobody: both account channels are off, so the
  // notifier skips this recipient before it ever looks at scope. A lit
  // "Near me" chip there is a delivery promise with nothing behind it.
  describe('when both account channels are off', () => {
    const paused = (scope: FollowAlertScope | undefined = 'near_me') => {
      mockAlerts = {
        entity_type: 'artist',
        entity_id: 7,
        shows: { enabled: true, in_app: false, email: false, scope },
      }
    }

    it('says paused, and stops the chips claiming to be delivering', () => {
      paused()
      renderArtist()

      expect(screen.getByText('paused')).toBeInTheDocument()
      expect(
        screen.getByRole('radiogroup', { name: 'Alerts for Just Mustard, paused' })
      ).toBeInTheDocument()
      expect(screen.getByText('While paused:')).toBeInTheDocument()
      expect(screen.queryByText('Alerts:')).toBeNull()
    })

    // The chips STAY. Swapping them for a link unmounted the node focus
    // returns to, dropping a keyboard user to <body> mid-commit, and it
    // removed the only way to switch a paused follow off.
    it('keeps the choice usable, including switching the follow off', async () => {
      const user = userEvent.setup()
      paused()
      renderArtist()

      await user.click(screen.getByRole('radio', { name: 'Off' }))

      expect(mockUpdate).toHaveBeenCalledWith({
        entityType: 'artists',
        entityId: 7,
        update: { shows: { enabled: false } },
      })
    })

    it('points at the alert matrix, which is where a channel lives', () => {
      paused()
      renderArtist()

      expect(
        screen.getByRole('link', { name: /paused.*alert settings/i })
      ).toHaveAttribute('href', '/profile?tab=settings#alerts')
    })

    // The whole point of not writing anything on its own: the scope is still
    // stored, so switching a channel back on restores near me rather than
    // asking for it again.
    it('writes nothing on its own, so the stored scope survives the pause', () => {
      paused()
      renderArtist()

      expect(mockUpdate).not.toHaveBeenCalled()
    })

    // A venue's subscription has no scope to preserve, but the delivery
    // promise is identical, and two surfaces over one field must not disagree
    // about whether it is being kept.
    it('reads paused on a venue too', () => {
      mockAlerts = {
        entity_type: 'venue',
        entity_id: 4,
        shows: { enabled: true, in_app: false, email: false },
      }
      renderVenue()

      expect(screen.getByText('paused')).toBeInTheDocument()
      expect(screen.getByRole('radio', { name: 'On' })).toBeInTheDocument()
    })

    // OFF is a choice made on this follow, and its control still works.
    // Only the enabled-but-undeliverable state is a pause.
    it('leaves a switched-off follow on the chips', () => {
      mockAlerts = {
        entity_type: 'artist',
        entity_id: 7,
        shows: { enabled: false, in_app: false, email: false, scope: 'near_me' },
      }
      renderArtist()

      expect(screen.getByRole('radiogroup')).toBeInTheDocument()
      expect(screen.queryByText('paused')).toBeNull()
    })
  })
})
