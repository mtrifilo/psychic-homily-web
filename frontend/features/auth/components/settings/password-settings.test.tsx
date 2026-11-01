import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { buildReauthHref } from '@/lib/auth-href'
import { sanitizeReturnTo } from '@/app/auth/auth-redirect-utils'
import {
  PasswordSettings,
  PASSWORD_SETTINGS_ANCHOR,
  PASSWORD_SETTINGS_HREF,
} from './password-settings'

type Profile = {
  success: boolean
  user?: { oauth_accounts?: { provider: string }[] | null }
  has_password?: boolean
  passkey_count?: number
}

let mockProfile: Profile | undefined
const mockSetMutate = vi.fn()

vi.mock('@/features/auth', () => ({
  useProfile: () => ({ data: mockProfile }),
  useSetPassword: () => ({
    mutate: mockSetMutate,
    isPending: false,
    isError: false,
    error: null,
  }),
  useChangePassword: () => ({
    mutate: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
}))

function googleOnly(overrides: Partial<Profile> = {}): Profile {
  return {
    success: true,
    user: { oauth_accounts: [{ provider: 'google' }] },
    has_password: false,
    passkey_count: 0,
    ...overrides,
  }
}

describe('PasswordSettings', () => {
  beforeEach(() => {
    mockSetMutate.mockReset()
    mockProfile = undefined
  })

  it('offers Set a password when the profile says the account has none', () => {
    mockProfile = googleOnly()
    renderWithProviders(<PasswordSettings />)

    expect(screen.getByRole('button', { name: 'Set a password' })).toBeInTheDocument()
    expect(screen.getByText('You sign in with Google. This account has no password.')).toBeInTheDocument()
    expect(screen.queryByText('Change password')).toBeNull()
  })

  it.each([
    ['the account has a password', googleOnly({ has_password: true })],
    ['the profile omits has_password', googleOnly({ has_password: undefined })],
    ['the profile has not loaded', undefined],
    ['the profile read failed', { success: false }],
  ])('keeps Change password when %s', (_name, profile) => {
    mockProfile = profile
    renderWithProviders(<PasswordSettings />)

    expect(screen.getByText('Change password')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Set a password' })).toBeNull()
  })

  it('becomes Change password carrying the confirmation once the set succeeds', async () => {
    mockProfile = googleOnly()
    mockSetMutate.mockImplementation((_password: string, options?: { onSuccess?: () => void }) => {
      options?.onSuccess?.()
    })
    const user = userEvent.setup()
    renderWithProviders(<PasswordSettings />)

    await user.click(screen.getByRole('button', { name: 'Set a password' }))
    await user.type(screen.getByLabelText('New password'), 'Correct-Horse-Battery-9')
    await user.type(screen.getByLabelText('Confirm password'), 'Correct-Horse-Battery-9')
    await user.click(screen.getByRole('button', { name: 'Set password' }))

    await waitFor(() => expect(screen.getByText('Change password')).toBeInTheDocument())
    expect(
      screen.getByText('Password set. You can now also sign in with your email and password.')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Current password')).toBeInTheDocument()
  })

  it('is the anchor the return address names', () => {
    mockProfile = googleOnly()
    const { container } = renderWithProviders(<PasswordSettings />)

    expect(container.querySelector(`#${PASSWORD_SETTINGS_ANCHOR}`)).not.toBeNull()
    expect(PASSWORD_SETTINGS_HREF).toBe('/profile?tab=settings#password')
  })
})

// The re-auth link carries the card's address, and the auth page reads it back
// unchanged, so completing a sign-in lands on the Settings tab at this card.
describe('PASSWORD_SETTINGS_HREF round trip', () => {
  it('survives the re-auth link and the auth page sanitizer', () => {
    const href = buildReauthHref(PASSWORD_SETTINGS_HREF)
    const returnTo = new URL(href, 'https://example.test').searchParams.get('returnTo')

    expect(returnTo).toBe(PASSWORD_SETTINGS_HREF)
    expect(sanitizeReturnTo(returnTo)).toBe(PASSWORD_SETTINGS_HREF)
  })
})
