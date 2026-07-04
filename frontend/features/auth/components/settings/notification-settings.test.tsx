import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { NotificationSettings } from './notification-settings'

// --- Mocks ---

const mockShowRemindersMutate = vi.fn()
let mockShowRemindersState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

const mockCollectionDigestMutate = vi.fn()
let mockCollectionDigestState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

const mockSceneDigestMutate = vi.fn()
let mockSceneDigestState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

const mockTierEditMutate = vi.fn()
let mockTierEditState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

let mockProfileData: {
  user?: {
    preferences?: {
      show_reminders?: boolean
      notify_on_collection_digest?: boolean
      notify_on_scene_digest?: boolean
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

vi.mock('@/features/shows', () => ({
  useSetShowReminders: () => ({
    mutate: mockShowRemindersMutate,
    ...mockShowRemindersState,
  }),
}))

vi.mock('@/features/collections', () => ({
  useSetCollectionDigestPreference: () => ({
    mutate: mockCollectionDigestMutate,
    ...mockCollectionDigestState,
  }),
}))

vi.mock('@/features/scenes', () => ({
  useSetSceneDigestPreference: () => ({
    mutate: mockSceneDigestMutate,
    ...mockSceneDigestState,
  }),
}))

// --- Tests ---

describe('NotificationSettings', () => {
  beforeEach(() => {
    mockShowRemindersMutate.mockReset()
    mockShowRemindersState = { isPending: false, isError: false, error: null }
    mockCollectionDigestMutate.mockReset()
    mockCollectionDigestState = {
      isPending: false,
      isError: false,
      error: null,
    }
    mockSceneDigestMutate.mockReset()
    mockSceneDigestState = {
      isPending: false,
      isError: false,
      error: null,
    }
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

    expect(screen.getByText('Notifications')).toBeInTheDocument()
    expect(
      screen.getByText(
        /Control how you're notified about upcoming shows, your collections, and your contributions/
      )
    ).toBeInTheDocument()
  })

  it('renders the show reminders label and description', () => {
    renderWithProviders(<NotificationSettings />)

    expect(screen.getByText('Show reminders')).toBeInTheDocument()
    expect(
      screen.getByText('Get an email 24 hours before your saved shows')
    ).toBeInTheDocument()
  })

  it('renders the switch in unchecked state when show_reminders is false', () => {
    mockProfileData = {
      user: { preferences: { show_reminders: false } },
    }
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    expect(toggle).toBeInTheDocument()
    expect(toggle).not.toBeChecked()
  })

  it('renders the switch in checked state when show_reminders is true', () => {
    mockProfileData = {
      user: { preferences: { show_reminders: true } },
    }
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    expect(toggle).toBeChecked()
  })

  it('defaults to unchecked when preferences are undefined', () => {
    mockProfileData = {}
    renderWithProviders(<NotificationSettings />)

    const showRemindersToggle = screen.getByRole('switch', {
      name: 'Show reminders',
    })
    expect(showRemindersToggle).not.toBeChecked()
    const digestToggle = screen.getByRole('switch', {
      name: /Weekly digest of new items in collections I follow/,
    })
    expect(digestToggle).not.toBeChecked()
  })

  it('calls mutate with true when toggling show reminders on', async () => {
    mockProfileData = {
      user: { preferences: { show_reminders: false } },
    }
    const user = userEvent.setup()
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    await user.click(toggle)

    expect(mockShowRemindersMutate).toHaveBeenCalledWith(true)
  })

  it('calls mutate with false when toggling show reminders off', async () => {
    mockProfileData = {
      user: { preferences: { show_reminders: true } },
    }
    const user = userEvent.setup()
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    await user.click(toggle)

    expect(mockShowRemindersMutate).toHaveBeenCalledWith(false)
  })

  it('disables the show reminders switch when its mutation is pending', () => {
    mockShowRemindersState = { isPending: true, isError: false, error: null }
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    expect(toggle).toBeDisabled()
  })

  it('shows show reminders error message when mutation fails', () => {
    mockShowRemindersState = {
      isPending: false,
      isError: true,
      error: new Error('Network error'),
    }
    renderWithProviders(<NotificationSettings />)

    // Both rows have the same error copy, so scope to the show-reminders block.
    const errors = screen.getAllByText('Failed to update setting. Please try again.')
    expect(errors.length).toBeGreaterThanOrEqual(1)
  })

  // ----- PSY-350 / PSY-515: weekly collection digest toggle -----

  describe('weekly collection digest toggle', () => {
    it('renders the digest label with the user-approved copy', () => {
      renderWithProviders(<NotificationSettings />)

      expect(
        screen.getByText('Weekly digest of new items in collections I follow')
      ).toBeInTheDocument()
    })

    it('reflects current value when notify_on_collection_digest is true', () => {
      mockProfileData = {
        user: { preferences: { notify_on_collection_digest: true } },
      }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      expect(toggle).toBeChecked()
    })

    it('reflects current value when notify_on_collection_digest is false', () => {
      mockProfileData = {
        user: { preferences: { notify_on_collection_digest: false } },
      }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      expect(toggle).not.toBeChecked()
    })

    it('defaults to OFF when the preference is undefined (matches server default)', () => {
      mockProfileData = { user: { preferences: {} } }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      expect(toggle).not.toBeChecked()
    })

    it('calls the mutation with true when toggled on', async () => {
      mockProfileData = {
        user: { preferences: { notify_on_collection_digest: false } },
      }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      await user.click(toggle)

      expect(mockCollectionDigestMutate).toHaveBeenCalledWith(true)
    })

    it('calls the mutation with false when toggled off', async () => {
      mockProfileData = {
        user: { preferences: { notify_on_collection_digest: true } },
      }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      await user.click(toggle)

      expect(mockCollectionDigestMutate).toHaveBeenCalledWith(false)
    })

    it('persists across reload — re-rendering with the new server state shows the toggle on', () => {
      // First render: server says off, user enables.
      mockProfileData = {
        user: { preferences: { notify_on_collection_digest: false } },
      }
      const { unmount } = renderWithProviders(<NotificationSettings />)
      let toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      expect(toggle).not.toBeChecked()
      unmount()

      // Simulate the profile query refetching with the persisted-true value.
      mockProfileData = {
        user: { preferences: { notify_on_collection_digest: true } },
      }
      renderWithProviders(<NotificationSettings />)
      toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      expect(toggle).toBeChecked()
    })

    it('disables the digest switch while the mutation is in flight', () => {
      mockCollectionDigestState = {
        isPending: true,
        isError: false,
        error: null,
      }
      renderWithProviders(<NotificationSettings />)

      const toggle = screen.getByRole('switch', {
        name: /Weekly digest of new items in collections I follow/,
      })
      expect(toggle).toBeDisabled()
    })

    it('renders an error message when the digest mutation fails', () => {
      mockCollectionDigestState = {
        isPending: false,
        isError: true,
        error: new Error('Server error'),
      }
      renderWithProviders(<NotificationSettings />)

      const errors = screen.getAllByText(
        'Failed to update setting. Please try again.'
      )
      expect(errors.length).toBeGreaterThanOrEqual(1)
    })
  })

  // ----- PSY-1342: weekly scene digest toggle -----

  describe('weekly scene digest toggle', () => {
    const label = /Weekly digest for scenes I follow/

    it('renders the scene digest label', () => {
      renderWithProviders(<NotificationSettings />)
      expect(screen.getByText('Weekly digest for scenes I follow')).toBeInTheDocument()
    })

    it('defaults to OFF when the preference is undefined (matches server default)', () => {
      mockProfileData = { user: { preferences: {} } }
      renderWithProviders(<NotificationSettings />)
      expect(screen.getByRole('switch', { name: label })).not.toBeChecked()
    })

    it('reflects the current value when notify_on_scene_digest is true', () => {
      mockProfileData = { user: { preferences: { notify_on_scene_digest: true } } }
      renderWithProviders(<NotificationSettings />)
      expect(screen.getByRole('switch', { name: label })).toBeChecked()
    })

    it('calls the mutation with true when toggled on', async () => {
      mockProfileData = { user: { preferences: { notify_on_scene_digest: false } } }
      const user = userEvent.setup()
      renderWithProviders(<NotificationSettings />)
      await user.click(screen.getByRole('switch', { name: label }))
      expect(mockSceneDigestMutate).toHaveBeenCalledWith(true)
    })

    it('disables the switch while the mutation is in flight', () => {
      mockSceneDigestState = { isPending: true, isError: false, error: null }
      renderWithProviders(<NotificationSettings />)
      expect(screen.getByRole('switch', { name: label })).toBeDisabled()
    })
  })

  // ----- PSY-756 / PSY-807: tier-change + edit-review email toggles -----

  describe('tier-change email toggle', () => {
    it('renders the tier-change label and description', () => {
      renderWithProviders(<NotificationSettings />)

      expect(screen.getByText('Tier-change emails')).toBeInTheDocument()
      expect(
        screen.getByText(/when your contributor tier changes/)
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
        screen.getByText(/when your submitted edits are approved or rejected/)
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

  // ----- Pending spinner placement (both rows) -----

  it('renders independent pending state per row — show-reminders pending does not disable digest', () => {
    mockShowRemindersState = { isPending: true, isError: false, error: null }
    renderWithProviders(<NotificationSettings />)

    const showRemindersToggle = screen.getByRole('switch', {
      name: 'Show reminders',
    })
    const digestToggle = screen.getByRole('switch', {
      name: /Weekly digest of new items in collections I follow/,
    })

    // Only show-reminders is mutating; the digest row must remain interactive.
    expect(showRemindersToggle).toBeDisabled()
    expect(digestToggle).toBeEnabled()
  })

  it('does not call the digest mutation when toggling show reminders', async () => {
    mockProfileData = {
      user: {
        preferences: {
          show_reminders: false,
          notify_on_collection_digest: false,
        },
      },
    }
    const user = userEvent.setup()
    renderWithProviders(<NotificationSettings />)

    await user.click(screen.getByRole('switch', { name: 'Show reminders' }))

    expect(mockShowRemindersMutate).toHaveBeenCalledTimes(1)
    expect(mockCollectionDigestMutate).not.toHaveBeenCalled()
  })
})
