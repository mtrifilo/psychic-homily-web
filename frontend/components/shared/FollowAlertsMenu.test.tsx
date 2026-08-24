import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { FollowAlertsMenu } from './FollowAlertsMenu'
import type { FollowAlertScope, FollowAlertSettings } from '@/lib/types/follow'

// The Library row's `[ alerts: … ]` bracket (PSY-1905).

const mockUpdate = vi.fn()
let mockUpdateState = { isPending: false, isError: false }

vi.mock('@/lib/hooks/common/useFollowAlerts', () => ({
  useUpdateFollowAlerts: () => ({ mutate: mockUpdate, ...mockUpdateState }),
}))

const alertsFor = (
  scope: FollowAlertScope | undefined = 'near_me',
  enabled = true
): FollowAlertSettings => ({
  entity_type: 'artist',
  entity_id: 1,
  shows: { enabled, in_app: true, email: false, scope },
})

/** A venue's resolved settings: no `scope`, and no `releases` axis at all. */
const venueAlerts = (enabled = true): FollowAlertSettings => ({
  entity_type: 'venue',
  entity_id: 1,
  shows: { enabled, in_app: true, email: false },
})

const renderMenu = (props: Partial<Parameters<typeof FollowAlertsMenu>[0]> = {}) =>
  renderWithProviders(
    <FollowAlertsMenu
      entityType="artists"
      entityId={1}
      entityName="Alpha"
      alerts={alertsFor()}
      hasHomeMetro
      {...props}
    />
  )

describe('FollowAlertsMenu', () => {
  beforeEach(() => {
    mockUpdate.mockReset()
    mockUpdateState = { isPending: false, isError: false }
  })

  it('summarizes the current scope in the bracket', () => {
    renderMenu()
    expect(
      screen.getByRole('button', { name: 'Show alerts for Alpha: near me' })
    ).toBeInTheDocument()
  })

  // The server signals capability per row by populating `alerts`. A row
  // without one gets no bracket, rather than a disabled one implying the
  // subscription could be switched on.
  it('renders nothing for a row the server sent no subscription for', () => {
    const { container } = renderMenu({ alerts: undefined })
    expect(container).toBeEmptyDOMElement()
  })

  // Unknown is not "no area". Rendering here would label a near-me follow
  // "everywhere", overstating the reach of a subscription the server scopes.
  it('renders nothing while the home area is still unknown', () => {
    const { container } = renderMenu({ hasHomeMetro: undefined })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a venue row without waiting on the home area', () => {
    renderMenu({
      entityType: 'venues',
      hasHomeMetro: undefined,
      alerts: { ...alertsFor(undefined), entity_type: 'venue' },
    })
    expect(
      screen.getByRole('button', { name: 'Show alerts for Alpha: on' })
    ).toBeInTheDocument()
  })

  it('writes the chosen scope, pinning only the axes it decides', async () => {
    const user = userEvent.setup()
    renderMenu()

    await user.click(
      screen.getByRole('button', { name: 'Show alerts for Alpha: near me' })
    )
    await user.click(screen.getByRole('menuitem', { name: 'Off' }))

    expect(mockUpdate).toHaveBeenCalledWith({
      entityType: 'artists',
      entityId: 1,
      update: { shows: { enabled: false } },
    })
  })

  // This bracket abbreviates ONE axis. Left unqualified it would promise that
  // "off" also silences the follow's release alerts, which no option here
  // touches and which are an account-level setting.
  it('names the axis it controls rather than claiming all alerts', async () => {
    const user = userEvent.setup()
    renderMenu()

    const trigger = screen.getByRole('button', {
      name: 'Show alerts for Alpha: near me',
    })
    expect(trigger).toHaveAttribute(
      'title',
      expect.stringContaining('Release alerts are set in Settings')
    )

    await user.click(trigger)
    expect(screen.getByText('New shows')).toBeInTheDocument()
  })

  // A menu button is not a toggle: aria-pressed contradicts an "off" label and
  // collides with the aria-haspopup the trigger already carries.
  it('does not announce itself as a pressed toggle', () => {
    renderMenu({ alerts: alertsFor('near_me', false) })
    const trigger = screen.getByRole('button', {
      name: 'Show alerts for Alpha: off',
    })
    expect(trigger).not.toHaveAttribute('aria-pressed')
    expect(trigger).toHaveAttribute('aria-haspopup', 'menu')
  })

  // The row renders from its served payload, not the optimistic cache, so a
  // rollback is invisible: without an error the outcome of a failed write is
  // byte-identical to nothing happening.
  it('reports a failed write instead of reverting silently', () => {
    mockUpdateState = { isPending: false, isError: true }
    renderMenu()
    expect(screen.getByRole('alert')).toHaveTextContent(
      "Couldn't save that. Try again."
    )
  })

  // aria-disabled, NOT the disabled attribute, and for a reason specific to a
  // MENU trigger: Radix restores focus here when the menu closes, and focus()
  // on a disabled element is a no-op. Committing a choice by keyboard used to
  // drop focus to <body>, restarting the next Tab at the top of a list that
  // can run 50 rows deep. The chip group one file over states the same rule.
  it('parks the bracket while a write is in flight without ejecting focus', () => {
    mockUpdateState = { isPending: true, isError: false }
    renderMenu()

    const trigger = screen.getByRole('button', { name: /Show alerts for Alpha/ })
    expect(trigger).toHaveAttribute('aria-disabled', 'true')
    expect(trigger).not.toBeDisabled()

    trigger.focus()
    expect(trigger).toHaveFocus()
  })

  // A venue follow has no release axis at all: the server omits `releases`
  // from a venue's settings and 422s a PATCH that sends one. Pointing a venue
  // follower at a Settings row that can never fire for them is the
  // cross-surface disagreement this control exists to end.
  it('does not offer venue followers a release-alert setting they cannot have', () => {
    renderMenu({ entityType: 'venues', alerts: venueAlerts() })

    expect(
      screen.getByRole('button', { name: /Show alerts for Alpha/ })
    ).toHaveAttribute('title', expect.not.stringContaining('Release alerts'))
  })

  it('still points artist followers at their release setting', () => {
    renderMenu()

    expect(
      screen.getByRole('button', { name: /Show alerts for Alpha/ })
    ).toHaveAttribute('title', expect.stringContaining('Release alerts are set in Settings'))
  })

  // The entity-page twin's rule, on a row. Both account channels off means
  // this subscription is enabled and reaching nobody, so summarizing it as
  // "near me" is a delivery promise the notifier does not keep.
  describe('when both account channels are off', () => {
    const pausedAlerts = (): FollowAlertSettings => ({
      entity_type: 'artist',
      entity_id: 1,
      shows: { enabled: true, in_app: false, email: false, scope: 'near_me' },
    })

    it('summarizes the row as paused, not as its stored scope', () => {
      renderMenu({ alerts: pausedAlerts() })

      expect(screen.getByText('alerts: paused')).toBeInTheDocument()
      expect(screen.queryByText('alerts: near me')).toBeNull()
    })

    // A link, not a menu: every option in that menu writes a field that
    // changes nothing until a channel comes back, and the channel is an
    // account setting rather than anything this row owns.
    it('links to the alert matrix instead of opening a menu', async () => {
      renderMenu({ alerts: pausedAlerts() })

      const bracket = screen.getByRole('link', {
        name: /New-show alerts for Alpha: paused/i,
      })
      expect(bracket).toHaveAttribute('href', '/profile?tab=settings#alerts')

      await userEvent.click(bracket)
      expect(screen.queryByRole('menu')).toBeNull()
      expect(mockUpdate).not.toHaveBeenCalled()
    })

    // The bar above these rows derives its own paused line from these same
    // payloads and needs no home area, while the option list is undefined
    // until the area read resolves and permanently if it fails. Guarding on
    // options first printed the bar's paused line over a column of rows with
    // no bracket at all, in the exact window they promise to agree.
    it('renders without waiting on the home area, which the pause does not need', () => {
      renderMenu({ alerts: pausedAlerts(), hasHomeMetro: undefined })

      expect(screen.getByText('alerts: paused')).toBeInTheDocument()
    })

    // A venue has no scope axis, so promising its scope back would invent a
    // setting that follow never had. It does still have a delivery
    // disclosure, and pausing must not be how that disclosure disappears.
    it('tailors the explanation to a venue, scope promise and all', () => {
      renderMenu({
        entityType: 'venues',
        alerts: {
          entity_type: 'venue',
          entity_id: 1,
          shows: { enabled: true, in_app: false, email: false },
        },
      })

      const bracket = screen.getByRole('link', { name: /paused/i })
      expect(bracket).toHaveAttribute(
        'title',
        expect.not.stringContaining('scope you chose')
      )
      expect(bracket).toHaveAttribute(
        'title',
        expect.stringContaining('still being switched on')
      )
    })

    // OFF is a choice made on this follow and keeps its menu; only the
    // enabled-but-undeliverable state is a pause.
    it('leaves a switched-off row on its menu', () => {
      renderMenu({
        alerts: {
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: false, in_app: false, email: false },
        },
      })

      expect(screen.getByText('alerts: off')).toBeInTheDocument()
    })
  })
})
