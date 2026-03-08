import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { NotificationSettings } from './notification-settings'

// --- Mocks ---

const mockMutate = vi.fn()
let mockMutationState = {
  isPending: false,
  isError: false,
  error: null as Error | null,
}

let mockProfileData: {
  user?: {
    preferences?: {
      show_reminders?: boolean
    }
  }
} = {}

vi.mock('@/lib/hooks/useAuth', () => ({
  useProfile: () => ({
    data: mockProfileData,
  }),
}))

vi.mock('@/lib/hooks/useShowReminders', () => ({
  useSetShowReminders: () => ({
    mutate: mockMutate,
    ...mockMutationState,
  }),
}))

// --- Tests ---

describe('NotificationSettings', () => {
  beforeEach(() => {
    mockMutate.mockReset()
    mockMutationState = { isPending: false, isError: false, error: null }
    mockProfileData = {}
  })

  it('renders the card title and description', () => {
    renderWithProviders(<NotificationSettings />)

    expect(screen.getByText('Notifications')).toBeInTheDocument()
    expect(
      screen.getByText(/Control how you're notified about upcoming shows/)
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

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    expect(toggle).not.toBeChecked()
  })

  it('calls mutate with true when toggling on', async () => {
    mockProfileData = {
      user: { preferences: { show_reminders: false } },
    }
    const user = userEvent.setup()
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    await user.click(toggle)

    expect(mockMutate).toHaveBeenCalledWith(true)
  })

  it('calls mutate with false when toggling off', async () => {
    mockProfileData = {
      user: { preferences: { show_reminders: true } },
    }
    const user = userEvent.setup()
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    await user.click(toggle)

    expect(mockMutate).toHaveBeenCalledWith(false)
  })

  it('disables the switch when mutation is pending', () => {
    mockMutationState = { isPending: true, isError: false, error: null }
    renderWithProviders(<NotificationSettings />)

    const toggle = screen.getByRole('switch', { name: 'Show reminders' })
    expect(toggle).toBeDisabled()
  })

  it('shows error message when mutation fails', () => {
    mockMutationState = {
      isPending: false,
      isError: true,
      error: new Error('Network error'),
    }
    renderWithProviders(<NotificationSettings />)

    expect(
      screen.getByText('Failed to update setting. Please try again.')
    ).toBeInTheDocument()
  })

  it('does not show error message when mutation is successful', () => {
    mockMutationState = { isPending: false, isError: false, error: null }
    renderWithProviders(<NotificationSettings />)

    expect(
      screen.queryByText('Failed to update setting. Please try again.')
    ).not.toBeInTheDocument()
  })
})
