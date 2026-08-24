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

    // The TRIGGER STAYS A TRIGGER. Swapping it for a link unmounted the node
    // Radix restores focus to, so committing a choice by keyboard dropped
    // focus to <body>, and it removed the only way to switch a paused follow
    // off. Only what the bracket says changes.
    it('keeps its menu, so the row is still controllable while paused', async () => {
      renderMenu({ alerts: pausedAlerts() })

      await userEvent.click(
        screen.getByRole('button', { name: /Show alerts for Alpha: paused/i })
      )

      expect(screen.getByRole('menu')).toBeInTheDocument()
      expect(screen.getByRole('menuitem', { name: 'Off' })).toBeInTheDocument()
    })

    // Reachable, and previously broken. With the account channels off, picking
    // an ON option on a row that was switched off flips the row INTO paused.
    // The whole interaction runs here so RADIX does the focus restore: it
    // focuses the trigger on close, and when the paused branch swapped that
    // trigger for a link, it focused a node that no longer existed and the
    // next Tab restarted at the top of a list that can run 50 rows deep.
    it('writes the choice and keeps focus when the row flips into paused', async () => {
      const offAlerts: FollowAlertSettings = {
        entity_type: 'artist',
        entity_id: 1,
        shows: { enabled: false, in_app: false, email: false },
      }
      const { rerender } = renderMenu({ alerts: offAlerts })

      const trigger = screen.getByRole('button', {
        name: /Show alerts for Alpha: off/i,
      })
      await userEvent.click(trigger)
      await userEvent.click(screen.getByRole('menuitem', { name: 'Near me' }))

      expect(mockUpdate).toHaveBeenCalledWith({
        entityType: 'artists',
        entityId: 1,
        update: { shows: { enabled: true, scope: 'near_me' } },
      })

      // What the server resolves that write back to, with no channel on.
      rerender(
        <FollowAlertsMenu
          entityType="artists"
          entityId={1}
          entityName="Alpha"
          alerts={pausedAlerts()}
          hasHomeMetro
        />
      )

      expect(screen.getByText('alerts: paused')).toBeInTheDocument()
      expect(document.activeElement).toBe(trigger)
    })

    // The explanation is one node inside the menu, reachable by keyboard and
    // touch, rather than a paragraph repeated as the accessible name of every
    // row on the tab.
    it('explains the pause inside the menu and offers the way out there', async () => {
      renderMenu({ alerts: pausedAlerts() })

      await userEvent.click(
        screen.getByRole('button', { name: /Show alerts for Alpha: paused/i })
      )

      expect(screen.getByText(/New-show alerts are paused/i)).toBeInTheDocument()
      expect(
        screen.getByRole('menuitem', { name: 'Turn a channel on' })
      ).toHaveAttribute('href', '/profile?tab=settings#alerts')

      // The options stop claiming to be delivering and say what the setting
      // is while it waits. This relabel is the whole reason keeping the
      // control mounted stays honest.
      expect(screen.getByText('While paused:')).toBeInTheDocument()
      expect(screen.queryByText('New shows')).toBeNull()
    })

    // A name, not a description: 50 rows announcing the same paragraph makes
    // a links list unusable and buries each row's own name inside it.
    it('keeps the accessible name short, with the prose in the menu', () => {
      renderMenu({ alerts: pausedAlerts() })

      const trigger = screen.getByRole('button', {
        name: 'Show alerts for Alpha: paused',
      })
      expect(trigger).toBeInTheDocument()
    })

    // A venue has no scope axis, so promising its scope back would invent a
    // setting that follow never had. It does still have a delivery
    // disclosure, and pausing must not be how that disclosure disappears.
    it('tailors the explanation to a venue, scope promise and all', async () => {
      renderMenu({
        entityType: 'venues',
        alerts: {
          entity_type: 'venue',
          entity_id: 1,
          shows: { enabled: true, in_app: false, email: false },
        },
      })

      await userEvent.click(screen.getByRole('button', { name: /paused/i }))

      expect(screen.getByText(/still being switched on/i)).toBeInTheDocument()
      expect(screen.queryByText(/scope for this follow/i)).toBeNull()
      expect(screen.queryByText(/geography-scoped/i)).toBeNull()
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
