import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { PasskeySignupButton } from './passkey-signup'

const mockPush = vi.fn()
const mockSetUser = vi.fn()
const mockStartRegistration = vi.fn()
const fetchMock = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startRegistration: (...args: unknown[]) => mockStartRegistration(...args),
}))

vi.mock('./backup-auth-prompt', () => ({
  BackupAuthPrompt: ({
    open,
    onComplete,
  }: {
    open: boolean
    onComplete: () => void
  }) =>
    open ? (
      <button type="button" onClick={onComplete}>
        Complete backup setup
      </button>
    ) : null,
}))

describe('PasskeySignupButton', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockSetUser.mockReset()
    mockStartRegistration.mockReset()
    fetchMock.mockReset()

    mockStartRegistration.mockResolvedValue({ id: 'credential-id' })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('redirects to returnTo after backup auth completion', async () => {
    const user = userEvent.setup()

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        options: { challenge: 'begin-signup-challenge' },
        challenge_id: 'challenge-id',
      }),
    })

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        user: {
          id: 100,
          email: 'signup@example.com',
          first_name: 'Signup',
          last_name: 'User',
          email_verified: false,
        },
      }),
    })

    renderWithProviders(<PasskeySignupButton returnTo="/collection" />)

    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    await user.click(screen.getByRole('checkbox'))
    await user.click(
      screen.getByRole('button', { name: /continue with passkey/i })
    )

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })

    const beginSignupPayload = JSON.parse(
      (fetchMock.mock.calls[0]?.[1] as { body: string }).body
    )
    expect(beginSignupPayload).toMatchObject({
      email: 'signup@example.com',
      terms_accepted: true,
      terms_version: '2026-01-31',
      privacy_version: '2026-02-15',
    })

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /complete backup setup/i })
      ).toBeInTheDocument()
    })

    await user.click(
      screen.getByRole('button', { name: /complete backup setup/i })
    )

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/collection')
    })
  })
})
