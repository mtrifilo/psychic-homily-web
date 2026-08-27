import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { NotificationSettings } from './notification-settings'

// PSY-1905 moved the show-reminder and both weekly-digest rows out of this
// card and into the Alerts matrix. Their coverage moved with them, to
// `alert-settings.test.tsx`. It was not dropped. What is left here is the
// mail the site sends ABOUT YOUR ACCOUNT, which stayed put.

// --- Mocks ---

const mockTierEditMutate = vi.fn()
let mockTierEditState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

let mockProfileData: {
  user?: {
    preferences?: {
      notify_on_tier_notifications?: boolean
      notify_on_edit_notifications?: boolean
    }
  }
} = {}

vi.mock('@/features/auth', () => ({
  useProfile: () => ({
    data: mockProfileData,
  }),
  useSetTierEditNotificationPreference: () => ({
    mutate: mockTierEditMutate,
    ...mockTierEditState,
  }),
}))

// --- Tests ---

describe('NotificationSettings', () => {
  beforeEach(() => {
    mockTierEditMutate.mockReset()
    mockTierEditState = {
      isPending: false,
      isError: false,
      error: null,
    }
    mockProfileData = {}
  })

  it('renders the card title and description', () => {
    renderWithProviders(<NotificationSettings />)

    expect(screen.getByText('Account emails')).toBeInTheDocument()
    expect(
      screen.getByText(/Mail about your account rather than about the index/)
    ).toBeInTheDocument()
  })

  // The whole point of the move: one boolean, one control. A copy left behind
  // here would let the two disagree.
  it('no longer renders the rows that moved into the Alerts matrix', () => {
    renderWithProviders(<NotificationSettings />)

    expect(
      screen.queryByRole('switch', { name: 'Show reminders' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('switch', { name: /Weekly digest for scenes I follow/ })
    ).not.toBeInTheDocument()
  })

  // ----- PSY-756 / PSY-807: tier-change + edit-review email toggles -----

  describe('tier-change email toggle', () => {
    it('renders the tier-change label and description', () => {
      renderWithProviders(<NotificationSettings />)

      expect(screen.getByText('Tier-change emails')).toBeInTheDocument()
      expect(
        screen.getByText(/When your contributor tier advances/)
      ).toBeInTheDocument()
    })

    it('defaults to ON when the preference is undefined (opt-OUT default)', () => {
      mockProfileData = { user: { preferences: {} } }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', { name: 'Tier-change emails' })
      expect(toggle).toBeChecked()
    })

    it('reflects current value when notify_on_tier_notifications is false', () => {
      mockProfileData = {
        user: { preferences: { notify_on_tier_notifications: false } },
      }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', { name: 'Tier-change emails' })
      expect(toggle).not.toBeChecked()
    })

    it('calls the mutation with only the tier field when toggled off', async () => {
      mockProfileData = {
        user: { preferences: { notify_on_tier_notifications: true } },
      }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)

      await user.click(
        screen.getByRole('switch', { name: 'Tier-change emails' })
      )

      expect(mockTierEditMutate).toHaveBeenCalledWith({
        notify_on_tier_notifications: false,
      })
    })

    it('calls the mutation with only the tier field when toggled back on', async () => {
      mockProfileData = {
        user: { preferences: { notify_on_tier_notifications: false } },
      }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)

      await user.click(
        screen.getByRole('switch', { name: 'Tier-change emails' })
      )

      expect(mockTierEditMutate).toHaveBeenCalledWith({
        notify_on_tier_notifications: true,
      })
    })

    it('persists across reload — re-rendering with the opted-out state shows it off', () => {
      mockProfileData = {
        user: { preferences: { notify_on_tier_notifications: true } },
      }
      const { unmount } = renderWithProviders(<NotificationSettings />)
      let toggle = screen.getByRole('switch', { name: 'Tier-change emails' })
      expect(toggle).toBeChecked()
      unmount()

      mockProfileData = {
        user: { preferences: { notify_on_tier_notifications: false } },
      }
      renderWithProviders(<NotificationSettings />)
      toggle = screen.getByRole('switch', { name: 'Tier-change emails' })
      expect(toggle).not.toBeChecked()
    })
  })

  describe('edit-review email toggle', () => {
    it('renders the edit-review label and description', () => {
      renderWithProviders(<NotificationSettings />)

      expect(screen.getByText('Edit-review emails')).toBeInTheDocument()
      expect(
        screen.getByText(/When a pending edit you submitted is reviewed/)
      ).toBeInTheDocument()
    })

    it('defaults to ON when the preference is undefined (opt-OUT default)', () => {
      mockProfileData = { user: { preferences: {} } }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', { name: 'Edit-review emails' })
      expect(toggle).toBeChecked()
    })

    it('reflects current value when notify_on_edit_notifications is false', () => {
      mockProfileData = {
        user: { preferences: { notify_on_edit_notifications: false } },
      }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', { name: 'Edit-review emails' })
      expect(toggle).not.toBeChecked()
    })

    it('calls the mutation with only the edit field when toggled off', async () => {
      mockProfileData = {
        user: { preferences: { notify_on_edit_notifications: true } },
      }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)

      await user.click(
        screen.getByRole('switch', { name: 'Edit-review emails' })
      )

      expect(mockTierEditMutate).toHaveBeenCalledWith({
        notify_on_edit_notifications: false,
      })
    })

    it('toggling one category does not send the other category field', async () => {
      mockProfileData = {
        user: {
          preferences: {
            notify_on_tier_notifications: true,
            notify_on_edit_notifications: true,
          },
        },
      }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)

      await user.click(
        screen.getByRole('switch', { name: 'Edit-review emails' })
      )

      expect(mockTierEditMutate).toHaveBeenCalledTimes(1)
      expect(mockTierEditMutate).toHaveBeenCalledWith({
        notify_on_edit_notifications: false,
      })
    })
  })

  it('disables both tier/edit switches while the shared mutation is in flight', () => {
    mockTierEditState = { isPending: true, isError: false, error: null }
    renderWithProviders(<NotificationSettings />)

    expect(
      screen.getByRole('switch', { name: 'Tier-change emails' })
    ).toBeDisabled()
    expect(
      screen.getByRole('switch', { name: 'Edit-review emails' })
    ).toBeDisabled()
  })

  it('shows an error message when the tier/edit mutation fails', () => {
    mockTierEditState = {
      isPending: false,
      isError: true,
      error: new Error('Server error'),
    }
    renderWithProviders(<NotificationSettings />)

    const errors = screen.getAllByText(
      'Failed to update setting. Please try again.'
    )
    // Both tier and edit rows render the shared error copy.
    expect(errors.length).toBeGreaterThanOrEqual(2)
  })
})
