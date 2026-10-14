import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { PasskeySignupButton } from './passkey-signup'

const mockSetUser = vi.fn()
const mockOnHandoffChange = vi.fn()
const mockOnError = vi.fn()
const mockStartRegistration = vi.fn()
const fetchMock = vi.fn()

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startRegistration: (...args: unknown[]) => mockStartRegistration(...args),
}))

// The components must resolve the backend through lib/api-base like every
// other data request, never from NEXT_PUBLIC_API_URL on their own: unset, that
// read skips the same-origin /api proxy.
vi.mock('@/lib/api-base', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  API_BASE_URL: 'https://api-base.test',
}))

describe('PasskeySignupButton', () => {
  beforeEach(() => {
    mockSetUser.mockReset()
    mockOnHandoffChange.mockReset()
    mockOnError.mockReset()
    mockStartRegistration.mockReset()
    fetchMock.mockReset()

    mockStartRegistration.mockResolvedValue({ id: 'credential-id' })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function renderButton() {
    renderWithProviders(
      <PasskeySignupButton
        onHandoffChange={mockOnHandoffChange}
        onError={mockOnError}
      />
    )
  }

  async function submitSignup() {
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    // Two required checkboxes: terms + age confirmation.
    await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))
    await user.click(screen.getByRole('checkbox', { name: /at least 16 years old/ }))
    await user.click(
      screen.getByRole('button', { name: /continue with passkey/i })
    )
  }

  const BEGIN_OK = {
    json: async () => ({
      success: true,
      options: { publicKey: { challenge: 'begin-signup-challenge' } },
      challenge_id: 'challenge-id',
    }),
  }

  it('hands the new session to the page: pending before finish, then complete', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
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

    renderButton()
    await submitSignup()

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
      age_confirmed: true,
      min_age_attested: 16,
    })

    expect(mockStartRegistration).toHaveBeenCalledWith({
      optionsJSON: { challenge: 'begin-signup-challenge' },
    })
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      'https://api-base.test/auth/passkey/signup/begin',
      'https://api-base.test/auth/passkey/signup/finish',
    ])

    const finishSignupPayload = JSON.parse(
      (fetchMock.mock.calls[1]?.[1] as { body: string }).body
    )
    expect(finishSignupPayload).toMatchObject({
      age_confirmed: true,
      min_age_attested: 16,
    })

    await waitFor(() => {
      expect(mockOnHandoffChange).toHaveBeenCalledTimes(2)
    })
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [{ status: 'complete', email: 'signup@example.com', method: 'passkey' }],
    ])
    expect(mockSetUser).toHaveBeenCalledWith(
      expect.objectContaining({ id: 100, email: 'signup@example.com' })
    )
    await waitFor(() => {
      expect(
        screen.queryByRole('dialog', { name: 'Create account with passkey' })
      ).not.toBeInTheDocument()
    })
    // The pending claim is in place before the finish request goes out.
    expect(mockOnHandoffChange.mock.invocationCallOrder[0]).toBeLessThan(
      fetchMock.mock.invocationCallOrder[1]
    )
  })

  it('releases its claim and reports an error when finish returns no user', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: true }),
    })

    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Failed to complete passkey signup')
    })
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
    expect(mockSetUser).not.toHaveBeenCalled()
  })

  it('releases its claim and reports the error when finish is refused', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Email already registered')
    })
    expect(mockOnHandoffChange.mock.calls).toEqual([
      [{ status: 'pending', email: 'signup@example.com' }],
      [null],
    ])
    expect(mockSetUser).not.toHaveBeenCalled()
  })

  it('claims nothing and reports the error when begin is refused', async () => {
    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: false, message: 'Email already registered' }),
    })

    renderButton()
    await submitSignup()

    await waitFor(() => {
      expect(mockOnError).toHaveBeenCalledWith('Email already registered')
    })
    expect(mockStartRegistration).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(mockOnHandoffChange).not.toHaveBeenCalled()
  })

  it('claims nothing when the passkey ceremony is cancelled', async () => {
    fetchMock.mockResolvedValueOnce(BEGIN_OK)
    mockStartRegistration.mockRejectedValueOnce(
      Object.assign(new Error('cancelled'), { name: 'NotAllowedError' })
    )

    renderButton()
    await submitSignup()

    // Re-enabled only once the catch and finally blocks have run.
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /continue with passkey/i })
      ).toBeEnabled()
    })
    expect(mockStartRegistration).toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(mockOnHandoffChange).not.toHaveBeenCalled()
    expect(mockOnError).not.toHaveBeenCalled()
  })

  // PSY-1023: the Continue button is disabled and no signup request fires until
  // the age confirmation is checked, even with email + terms provided.
  it('blocks passkey signup until age is confirmed', async () => {
    const user = userEvent.setup()

    renderButton()

    await user.click(screen.getByRole('button', { name: /sign up with passkey/i }))
    await user.type(screen.getByLabelText('Email'), 'signup@example.com')
    await user.click(screen.getByRole('checkbox', { name: /Terms of Service/ }))

    // Age unchecked → Continue button stays disabled and no fetch fires.
    expect(
      screen.getByRole('button', { name: /continue with passkey/i })
    ).toBeDisabled()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
