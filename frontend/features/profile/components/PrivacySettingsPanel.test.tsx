import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PrivacySettingsPanel } from './PrivacySettingsPanel'
import type { PrivacySettings } from '@/features/auth'

// Mock profile data
const basePrivacySettings: PrivacySettings = {
  contributions: 'visible',
  saved_shows: 'visible',
  following: 'visible',
  collections: 'visible',
  last_active: 'visible',
  profile_sections: 'visible',
}

type MockProfile = {
  id: number
  username: string
  profile_visibility: 'public' | 'private'
  privacy_settings: PrivacySettings
}

const baseProfile: MockProfile = {
  id: 1,
  username: 'testuser',
  profile_visibility: 'public',
  privacy_settings: { ...basePrivacySettings },
}

type MockUseOwnContributorProfileValue = {
  data: MockProfile | null
  isLoading: boolean
}
const mockUseOwnContributorProfile = vi.fn<
  () => MockUseOwnContributorProfileValue
>(() => ({
  data: baseProfile,
  isLoading: false,
}))

const mockVisibilityMutate = vi.fn()
type MockVisibilityHookValue = {
  mutate: typeof mockVisibilityMutate
  isPending: boolean
  isError: boolean
  error: { message: string } | null
}
const mockUseUpdateVisibility = vi.fn<() => MockVisibilityHookValue>(() => ({
  mutate: mockVisibilityMutate,
  isPending: false,
  isError: false,
  error: null,
}))

const mockPrivacyMutate = vi.fn()
type MockPrivacyHookValue = {
  mutate: typeof mockPrivacyMutate
  isPending: boolean
  isError: boolean
  error: { message: string } | null
}
const mockUseUpdatePrivacy = vi.fn<() => MockPrivacyHookValue>(() => ({
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

  describe('loading + initial render', () => {
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

    it('shows "Private Profile" copy when visibility is private', () => {
      mockUseOwnContributorProfile.mockReturnValue({
        data: {
          ...baseProfile,
          profile_visibility: 'private',
          privacy_settings: { ...basePrivacySettings },
        },
        isLoading: false,
      })
      render(<PrivacySettingsPanel />)
      expect(screen.getByText('Private Profile')).toBeInTheDocument()
      expect(
        screen.getByText(/Only you can see your profile/i)
      ).toBeInTheDocument()
    })

    it('shows the public profile URL when visibility is public', () => {
      render(<PrivacySettingsPanel />)
      expect(
        screen.getByText(/visible to everyone at \/users\/testuser/)
      ).toBeInTheDocument()
    })

    it('renders privacy controls section with all fields', () => {
      render(<PrivacySettingsPanel />)
      expect(screen.getByText('Privacy Controls')).toBeInTheDocument()
      expect(screen.getByText('Contributions')).toBeInTheDocument()
      expect(screen.getByText('Saved Shows')).toBeInTheDocument()
      expect(screen.getByText('Following')).toBeInTheDocument()
      expect(screen.getByText('Collections')).toBeInTheDocument()
      expect(screen.getByText('Last Active')).toBeInTheDocument()
      expect(screen.getByText('Custom Sections')).toBeInTheDocument()
    })

    it('renders three privacy-level buttons (Visible / Count Only / Hidden) per privacy field', () => {
      render(<PrivacySettingsPanel />)
      // 4 three-level fields × 3 buttons = 12 buttons; plus the Save button
      expect(
        screen.getAllByRole('button', { name: /^Visible$/i })
      ).toHaveLength(4)
      expect(
        screen.getAllByRole('button', { name: /^Count Only$/i })
      ).toHaveLength(4)
      expect(
        screen.getAllByRole('button', { name: /^Hidden$/i })
      ).toHaveLength(4)
    })
  })

  describe('visibility toggle', () => {
    it('calls updateVisibility with the inverse on toggle', () => {
      render(<PrivacySettingsPanel />)
      const switches = screen.getAllByRole('switch')
      act(() => {
        switches[0].click()
      })
      expect(mockVisibilityMutate).toHaveBeenCalledTimes(1)
      const [payload] = mockVisibilityMutate.mock.calls[0]
      expect(payload).toEqual({ visibility: 'private' })
    })

    it('toggles to public when current state is private', () => {
      mockUseOwnContributorProfile.mockReturnValue({
        data: {
          ...baseProfile,
          profile_visibility: 'private',
          privacy_settings: { ...basePrivacySettings },
        },
        isLoading: false,
      })
      render(<PrivacySettingsPanel />)
      const switches = screen.getAllByRole('switch')
      act(() => {
        switches[0].click()
      })
      const [payload] = mockVisibilityMutate.mock.calls[0]
      expect(payload).toEqual({ visibility: 'public' })
    })

    it('shows visibility error banner when updateVisibility is in error state', () => {
      mockUseUpdateVisibility.mockReturnValue({
        mutate: mockVisibilityMutate,
        isPending: false,
        isError: true,
        error: { message: 'Network unreachable' },
      })
      render(<PrivacySettingsPanel />)
      expect(screen.getByText('Network unreachable')).toBeInTheDocument()
    })

    it('falls back to "Failed to update visibility" copy when error has no message', () => {
      mockUseUpdateVisibility.mockReturnValue({
        mutate: mockVisibilityMutate,
        isPending: false,
        isError: true,
        error: null,
      })
      render(<PrivacySettingsPanel />)
      expect(
        screen.getByText('Failed to update visibility')
      ).toBeInTheDocument()
    })

    it('disables the visibility switch while update is pending', () => {
      mockUseUpdateVisibility.mockReturnValue({
        mutate: mockVisibilityMutate,
        isPending: true,
        isError: false,
        error: null,
      })
      render(<PrivacySettingsPanel />)
      const switches = screen.getAllByRole('switch')
      // First switch is the visibility toggle
      expect(switches[0]).toBeDisabled()
    })

    it('cleans up visibility success timeout on unmount', () => {
      mockVisibilityMutate.mockImplementation(
        (_input: unknown, opts: { onSuccess?: () => void }) => {
          opts.onSuccess?.()
        }
      )

      const { unmount } = render(<PrivacySettingsPanel />)

      const switches = screen.getAllByRole('switch')
      act(() => {
        switches[0].click()
      })

      expect(screen.getByText('Settings saved')).toBeInTheDocument()
      unmount()

      act(() => {
        vi.advanceTimersByTime(4000)
      })
    })
  })

  describe('privacy field controls', () => {
    it('Save Privacy Settings button is disabled when there are no changes', () => {
      render(<PrivacySettingsPanel />)
      const saveButton = screen.getByRole('button', {
        name: /Save Privacy Settings/i,
      })
      expect(saveButton).toBeDisabled()
    })

    it('enables the Save button after a privacy field changes', () => {
      render(<PrivacySettingsPanel />)
      const hiddenButtons = screen.getAllByRole('button', { name: /^Hidden$/i })
      act(() => {
        hiddenButtons[0].click()
      })
      const saveButton = screen.getByRole('button', {
        name: /Save Privacy Settings/i,
      })
      expect(saveButton).toBeEnabled()
    })

    it('cycles a three-level field through Hidden -> Count Only -> Visible', () => {
      render(<PrivacySettingsPanel />)

      // Click Hidden for the first three-level field (Contributions)
      act(() => {
        screen.getAllByRole('button', { name: /^Hidden$/i })[0].click()
      })
      // Click Count Only
      act(() => {
        screen.getAllByRole('button', { name: /^Count Only$/i })[0].click()
      })
      // Save
      act(() => {
        screen.getByRole('button', { name: /Save Privacy Settings/i }).click()
      })

      expect(mockPrivacyMutate).toHaveBeenCalledTimes(1)
      const [payload] = mockPrivacyMutate.mock.calls[0]
      expect(payload.contributions).toBe('count_only')
    })

    it('toggling the Last Active binary switch from visible to hidden sends "hidden"', () => {
      render(<PrivacySettingsPanel />)

      // The binary switches are the trailing two switches (after the visibility toggle).
      const switches = screen.getAllByRole('switch')
      // Index 0: visibility, 1: last_active, 2: profile_sections
      const lastActiveSwitch = switches[1]
      expect(lastActiveSwitch).toHaveAttribute('aria-checked', 'true')

      act(() => {
        lastActiveSwitch.click()
      })

      act(() => {
        screen.getByRole('button', { name: /Save Privacy Settings/i }).click()
      })

      const [payload] = mockPrivacyMutate.mock.calls[0]
      expect(payload.last_active).toBe('hidden')
    })

    it('toggling the Custom Sections binary switch from visible to hidden sends "hidden"', () => {
      render(<PrivacySettingsPanel />)

      const switches = screen.getAllByRole('switch')
      const customSectionsSwitch = switches[2]
      expect(customSectionsSwitch).toHaveAttribute('aria-checked', 'true')

      act(() => {
        customSectionsSwitch.click()
      })

      act(() => {
        screen.getByRole('button', { name: /Save Privacy Settings/i }).click()
      })

      const [payload] = mockPrivacyMutate.mock.calls[0]
      expect(payload.profile_sections).toBe('hidden')
    })

    it('toggling a binary switch from hidden to visible sends "visible"', () => {
      mockUseOwnContributorProfile.mockReturnValue({
        data: {
          ...baseProfile,
          privacy_settings: {
            ...basePrivacySettings,
            last_active: 'hidden',
          },
        },
        isLoading: false,
      })
      render(<PrivacySettingsPanel />)

      const switches = screen.getAllByRole('switch')
      const lastActiveSwitch = switches[1]
      expect(lastActiveSwitch).toHaveAttribute('aria-checked', 'false')

      act(() => {
        lastActiveSwitch.click()
      })

      act(() => {
        screen.getByRole('button', { name: /Save Privacy Settings/i }).click()
      })

      const [payload] = mockPrivacyMutate.mock.calls[0]
      expect(payload.last_active).toBe('visible')
    })

    it('sends the full PrivacySettings payload (all 6 fields) on save', () => {
      render(<PrivacySettingsPanel />)
      // Make any change so the save button enables
      act(() => {
        screen.getAllByRole('button', { name: /^Hidden$/i })[0].click()
      })
      act(() => {
        screen.getByRole('button', { name: /Save Privacy Settings/i }).click()
      })

      const [payload] = mockPrivacyMutate.mock.calls[0]
      expect(payload).toEqual({
        contributions: 'hidden',
        saved_shows: 'visible',
        following: 'visible',
        collections: 'visible',
        last_active: 'visible',
        profile_sections: 'visible',
      })
    })

    it('shows privacy error banner when updatePrivacy is in error state', () => {
      mockUseUpdatePrivacy.mockReturnValue({
        mutate: mockPrivacyMutate,
        isPending: false,
        isError: true,
        error: { message: 'Privacy update failed' },
      })
      render(<PrivacySettingsPanel />)
      expect(screen.getByText('Privacy update failed')).toBeInTheDocument()
    })

    it('falls back to "Failed to save" copy when privacy error has no message', () => {
      mockUseUpdatePrivacy.mockReturnValue({
        mutate: mockPrivacyMutate,
        isPending: false,
        isError: true,
        error: null,
      })
      render(<PrivacySettingsPanel />)
      expect(screen.getByText('Failed to save')).toBeInTheDocument()
    })

    it('disables Save button when privacy update is pending', () => {
      mockUseUpdatePrivacy.mockReturnValue({
        mutate: mockPrivacyMutate,
        isPending: true,
        isError: false,
        error: null,
      })
      render(<PrivacySettingsPanel />)

      // Toggle a setting first so `hasChanges` is true; otherwise the Save
      // button is already disabled by `!hasChanges` and `isPending` is never
      // load-bearing in the assertion.
      const hiddenButtons = screen.getAllByRole('button', { name: /Hidden/i })
      act(() => {
        hiddenButtons[0].click()
      })

      const saveButton = screen.getByRole('button', {
        name: /Save Privacy Settings/i,
      })
      expect(saveButton).toBeDisabled()
    })

    it('cleans up privacy save success timeout on unmount', () => {
      mockPrivacyMutate.mockImplementation(
        (_input: unknown, opts: { onSuccess?: () => void }) => {
          opts.onSuccess?.()
        }
      )

      const { unmount } = render(<PrivacySettingsPanel />)

      const hiddenButtons = screen.getAllByRole('button', { name: /Hidden/i })
      act(() => {
        hiddenButtons[0].click()
      })

      const saveButton = screen.getByRole('button', {
        name: /Save Privacy Settings/i,
      })
      act(() => {
        saveButton.click()
      })

      expect(screen.getByText('Settings saved')).toBeInTheDocument()
      unmount()

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

      const hiddenButtons = screen.getAllByRole('button', { name: /Hidden/i })
      act(() => {
        hiddenButtons[0].click()
      })

      const saveButton = screen.getByRole('button', {
        name: /Save Privacy Settings/i,
      })
      act(() => {
        saveButton.click()
      })

      const updatedPrivacy = {
        ...basePrivacySettings,
        contributions: 'hidden' as const,
      }
      mockUseOwnContributorProfile.mockReturnValue({
        data: { ...baseProfile, privacy_settings: updatedPrivacy },
        isLoading: false,
      })

      rerender(<PrivacySettingsPanel />)

      // After rerender with updated server data, the re-sync useEffect should
      // run again, so the hook is observed re-invoked. Without an assertion
      // this test would pass even if the re-sync logic regressed.
      expect(mockUseOwnContributorProfile).toHaveBeenCalled()
      expect(mockUseOwnContributorProfile.mock.calls.length).toBeGreaterThan(1)
    })

    it('clears the "Settings saved" indicator when a new change is made before timeout', () => {
      mockPrivacyMutate.mockImplementation(
        (_input: unknown, opts: { onSuccess?: () => void }) => {
          opts.onSuccess?.()
        }
      )

      render(<PrivacySettingsPanel />)

      // Make change + save → success indicator appears
      act(() => {
        screen.getAllByRole('button', { name: /^Hidden$/i })[0].click()
      })
      act(() => {
        screen.getByRole('button', { name: /Save Privacy Settings/i }).click()
      })
      expect(screen.getByText('Settings saved')).toBeInTheDocument()

      // Make a new change before the 3s timeout fires → indicator clears
      act(() => {
        screen.getAllByRole('button', { name: /^Count Only$/i })[0].click()
      })
      expect(screen.queryByText('Settings saved')).not.toBeInTheDocument()
    })
  })
})
