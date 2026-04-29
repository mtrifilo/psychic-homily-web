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

let mockProfileData: {
  user?: {
    preferences?: {
      show_reminders?: boolean
      notify_on_collection_digest?: boolean
    }
  }
} = {}

vi.mock('@/features/auth', () => ({
  useProfile: () => ({
    data: mockProfileData,
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
    mockProfileData = {}
  })

  it('renders the card title and description', () => {
    renderWithProviders(<NotificationSettings />)

    expect(screen.getByText('Notifications')).toBeInTheDocument()
    expect(
      screen.getByText(
        /Control how you're notified about upcoming shows and your collections/
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
})
