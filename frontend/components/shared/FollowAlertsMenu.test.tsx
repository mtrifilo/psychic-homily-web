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
})
