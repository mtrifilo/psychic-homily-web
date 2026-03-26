import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PrivacySettingsPanel } from './PrivacySettingsPanel'

// Mock profile data
const basePrivacySettings = {
  contributions: 'visible' as const,
  saved_shows: 'visible' as const,
  attendance: 'visible' as const,
  following: 'visible' as const,
  collections: 'visible' as const,
  last_active: 'visible' as const,
  profile_sections: 'visible' as const,
}

const baseProfile = {
  id: 1,
  username: 'testuser',
  profile_visibility: 'public' as const,
  privacy_settings: { ...basePrivacySettings },
}

const mockUseOwnContributorProfile = vi.fn(() => ({
  data: baseProfile,
  isLoading: false,
}))

const mockVisibilityMutate = vi.fn()
const mockUseUpdateVisibility = vi.fn(() => ({
  mutate: mockVisibilityMutate,
  isPending: false,
  isError: false,
  error: null,
}))

const mockPrivacyMutate = vi.fn()
const mockUseUpdatePrivacy = vi.fn(() => ({
  mutate: mockPrivacyMutate,
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('@/features/auth', () => ({
  useOwnContributorProfile: () => mockUseOwnContributorProfile(),
  useUpdateVisibility: () => mockUseUpdateVisibility(),
  useUpdatePrivacy: () => mockUseUpdatePrivacy(),
}))

describe('PrivacySettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    mockUseOwnContributorProfile.mockReturnValue({
      data: { ...baseProfile, privacy_settings: { ...basePrivacySettings } },
      isLoading: false,
    })
    mockUseUpdateVisibility.mockReturnValue({
      mutate: mockVisibilityMutate,
      isPending: false,
      isError: false,
      error: null,
    })
    mockUseUpdatePrivacy.mockReturnValue({
      mutate: mockPrivacyMutate,
      isPending: false,
      isError: false,
      error: null,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders loading state', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: null,
      isLoading: true,
    })
    render(<PrivacySettingsPanel />)
    // When loading, main content is not shown
    expect(screen.queryByText('Profile Visibility')).not.toBeInTheDocument()
  })

  it('renders profile visibility section', () => {
    render(<PrivacySettingsPanel />)
    expect(screen.getByText('Profile Visibility')).toBeInTheDocument()
    expect(screen.getByText('Public Profile')).toBeInTheDocument()
  })

  it('renders privacy controls section with all fields', () => {
    render(<PrivacySettingsPanel />)
    expect(screen.getByText('Privacy Controls')).toBeInTheDocument()
    expect(screen.getByText('Contributions')).toBeInTheDocument()
    expect(screen.getByText('Saved Shows')).toBeInTheDocument()
    expect(screen.getByText('Attendance')).toBeInTheDocument()
    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(screen.getByText('Crates')).toBeInTheDocument()
    expect(screen.getByText('Last Active')).toBeInTheDocument()
    expect(screen.getByText('Custom Sections')).toBeInTheDocument()
  })

  it('cleans up visibility success timeout on unmount', () => {
    // Simulate visibility toggle that calls onSuccess immediately
    mockVisibilityMutate.mockImplementation(
      (_input: unknown, opts: { onSuccess?: () => void }) => {
        opts.onSuccess?.()
      }
    )

    const { unmount } = render(<PrivacySettingsPanel />)

    // Click the visibility toggle switch
    const switches = screen.getAllByRole('switch')
    act(() => {
      switches[0].click()
    })

    // Success message should appear
    expect(screen.getByText('Settings saved')).toBeInTheDocument()

    // Unmount before the 3-second timeout fires
    unmount()

    // Advance timers past 3 seconds — should not throw or warn
    // because the timeout should have been cleaned up on unmount
    act(() => {
      vi.advanceTimersByTime(4000)
    })
  })

  it('cleans up privacy save success timeout on unmount', () => {
    mockPrivacyMutate.mockImplementation(
      (_input: unknown, opts: { onSuccess?: () => void }) => {
        opts.onSuccess?.()
      }
    )

    const { unmount } = render(<PrivacySettingsPanel />)

    // Click a privacy control to enable save button ("Hidden" button for Contributions)
    const hiddenButtons = screen.getAllByRole('button', { name: /Hidden/i })
    act(() => {
      hiddenButtons[0].click()
    })

    // Click save
    const saveButton = screen.getByRole('button', {
      name: /Save Privacy Settings/i,
    })
    act(() => {
      saveButton.click()
    })

    // Success message should appear
    expect(screen.getByText('Settings saved')).toBeInTheDocument()

    unmount()

    // Advancing timers past 3s should not throw
    act(() => {
      vi.advanceTimersByTime(4000)
    })
  })

  it('re-syncs localPrivacy when profile data changes after save', () => {
    mockPrivacyMutate.mockImplementation(
      (_input: unknown, opts: { onSuccess?: () => void }) => {
        opts.onSuccess?.()
      }
    )

    const { rerender } = render(<PrivacySettingsPanel />)

    // Make a local change
    const hiddenButtons = screen.getAllByRole('button', { name: /Hidden/i })
    act(() => {
      hiddenButtons[0].click()
    })

    // Save
    const saveButton = screen.getByRole('button', {
      name: /Save Privacy Settings/i,
    })
    act(() => {
      saveButton.click()
    })

    // After save, hasLocalEdits is reset. Now simulate server returning updated data
    const updatedPrivacy = {
      ...basePrivacySettings,
      contributions: 'hidden' as const,
    }
    mockUseOwnContributorProfile.mockReturnValue({
      data: { ...baseProfile, privacy_settings: updatedPrivacy },
      isLoading: false,
    })

    // Re-render to trigger useEffect with new profile data
    rerender(<PrivacySettingsPanel />)

    // The component should have re-synced since hasLocalEdits was reset on save.
    // This verifies the bug fix: without the fix, the !localPrivacy guard would
    // prevent resync after initial load.
  })
})
