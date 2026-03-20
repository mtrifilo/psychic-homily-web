import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { PrivacySettingsPanel } from './PrivacySettingsPanel'
import type { PublicProfileResponse, PrivacySettings } from '@/features/auth'

// Mock hooks
const mockUseOwnContributorProfile = vi.fn()
const mockVisibilityMutate = vi.fn()
const mockPrivacyMutate = vi.fn()

vi.mock('@/features/auth', () => ({
  useOwnContributorProfile: () => mockUseOwnContributorProfile(),
  useUpdateVisibility: () => ({
    mutate: mockVisibilityMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
  useUpdatePrivacy: () => ({
    mutate: mockPrivacyMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const defaultPrivacy: PrivacySettings = {
  contributions: 'visible',
  saved_shows: 'visible',
  attendance: 'visible',
  following: 'visible',
  collections: 'visible',
  last_active: 'visible',
  profile_sections: 'visible',
}

function makeProfile(
  overrides: Partial<PublicProfileResponse> = {}
): PublicProfileResponse {
  return {
    username: 'testuser',
    profile_visibility: 'public',
    user_tier: 'contributor',
    joined_at: '2025-01-15T00:00:00Z',
    privacy_settings: { ...defaultPrivacy },
    ...overrides,
  }
}

describe('PrivacySettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders loading spinner when loading', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: null,
      isLoading: true,
    })

    const { container } = renderWithProviders(<PrivacySettingsPanel />)
    // Check for the animate-spin class on the Loader2 icon
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders profile visibility section', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'public' }),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    expect(screen.getByText('Profile Visibility')).toBeInTheDocument()
    expect(screen.getByText('Public Profile')).toBeInTheDocument()
  })

  it('shows "Private Profile" text when profile is private', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'private' }),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    expect(screen.getByText('Private Profile')).toBeInTheDocument()
    expect(screen.getByText('Only you can see your profile')).toBeInTheDocument()
  })

  it('shows public profile URL text when public', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        profile_visibility: 'public',
        username: 'alice',
      }),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    expect(
      screen.getByText(/Your profile is visible to everyone at \/users\/alice/)
    ).toBeInTheDocument()
  })

  it('renders privacy controls section', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    expect(screen.getByText('Privacy Controls')).toBeInTheDocument()
  })

  it('renders all privacy field labels', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    expect(screen.getByText('Contributions')).toBeInTheDocument()
    expect(screen.getByText('Saved Shows')).toBeInTheDocument()
    expect(screen.getByText('Attendance')).toBeInTheDocument()
    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(screen.getByText('Collections')).toBeInTheDocument()
    expect(screen.getByText('Last Active')).toBeInTheDocument()
    expect(screen.getByText('Custom Sections')).toBeInTheDocument()
  })

  it('renders three-level privacy buttons for contributions', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    // There should be Visible/Count Only/Hidden buttons for each three-level field (5 fields)
    const visibleButtons = screen.getAllByText('Visible')
    expect(visibleButtons.length).toBe(5) // 5 three-level fields
    const countOnlyButtons = screen.getAllByText('Count Only')
    expect(countOnlyButtons.length).toBe(5)
    const hiddenButtons = screen.getAllByText('Hidden')
    expect(hiddenButtons.length).toBe(5)
  })

  it('toggles visibility when switch is changed', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'public' }),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)

    // The visibility switch is identified by the Switch component
    const switches = screen.getAllByRole('switch')
    // First switch is the profile visibility toggle
    fireEvent.click(switches[0])

    expect(mockVisibilityMutate).toHaveBeenCalledWith(
      { visibility: 'private' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('toggles from private to public', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'private' }),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)

    const switches = screen.getAllByRole('switch')
    fireEvent.click(switches[0])

    expect(mockVisibilityMutate).toHaveBeenCalledWith(
      { visibility: 'public' },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('enables save button after changing a privacy setting', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)

    const saveButton = screen.getByText('Save Privacy Settings')
    expect(saveButton).toBeDisabled()

    // Click "Count Only" for the first field (Contributions)
    const countOnlyButtons = screen.getAllByText('Count Only')
    fireEvent.click(countOnlyButtons[0])

    expect(saveButton).not.toBeDisabled()
  })

  it('calls updatePrivacy on save', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)

    // Change a setting to enable the save button
    const hiddenButtons = screen.getAllByText('Hidden')
    fireEvent.click(hiddenButtons[0]) // Set contributions to hidden

    const saveButton = screen.getByText('Save Privacy Settings')
    fireEvent.click(saveButton)

    expect(mockPrivacyMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        contributions: 'hidden',
        saved_shows: 'visible',
        attendance: 'visible',
        following: 'visible',
        collections: 'visible',
        last_active: 'visible',
        profile_sections: 'visible',
      }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
  })

  it('does not render privacy fields when profile has no privacy_settings', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ privacy_settings: undefined }),
      isLoading: false,
    })

    renderWithProviders(<PrivacySettingsPanel />)
    // Privacy Controls heading should still appear
    expect(screen.getByText('Privacy Controls')).toBeInTheDocument()
    // But the individual field labels should not (since localPrivacy is null)
    expect(screen.queryAllByText('Visible')).toHaveLength(0)
  })
})
