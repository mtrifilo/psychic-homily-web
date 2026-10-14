import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import userEvent from '@testing-library/user-event'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { PasskeyLoginButton } from './passkey-login'

const mockPush = vi.fn()
const mockSetUser = vi.fn()
const mockStartAuthentication = vi.fn()
const fetchMock = vi.fn()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ setUser: mockSetUser }),
}))

vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: vi.fn(() => true),
  startAuthentication: (...args: unknown[]) => mockStartAuthentication(...args),
}))

// The components must resolve the backend through lib/api-base like every
// other data request, never from NEXT_PUBLIC_API_URL on their own: unset, that
// read skips the same-origin /api proxy.
vi.mock('@/lib/api-base', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api-base')>()),
  API_BASE_URL: 'https://api-base.test',
}))

describe('PasskeyLoginButton', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockSetUser.mockReset()
    mockStartAuthentication.mockReset()
    fetchMock.mockReset()

    mockStartAuthentication.mockResolvedValue({ id: 'assertion-id' })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('redirects to returnTo after successful passkey login', async () => {
    const user = userEvent.setup()

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        options: {
          publicKey: { challenge: 'begin-challenge', rpId: 'localhost' },
        },
        challenge_id: 'challenge-id',
      }),
    })

    fetchMock.mockResolvedValueOnce({
      json: async () => ({
        success: true,
        user: {
          id: 42,
          email: 'test@example.com',
          first_name: 'Test',
          last_name: 'User',
          email_verified: true,
          is_admin: false,
        },
      }),
    })

    renderWithProviders(<PasskeyLoginButton returnTo="/library" />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/library')
    })
    // The begin endpoint wraps the options as the credentials.get() argument;
    // the library takes only the inner options.
    expect(mockStartAuthentication).toHaveBeenCalledWith({
      optionsJSON: { challenge: 'begin-challenge', rpId: 'localhost' },
    })
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      'https://api-base.test/auth/passkey/login/begin',
      'https://api-base.test/auth/passkey/login/finish',
    ])
  })

  it('reports a begin response without options instead of starting the ceremony', async () => {
    const user = userEvent.setup()
    const onError = vi.fn()

    fetchMock.mockResolvedValueOnce({
      json: async () => ({ success: true, challenge_id: 'challenge-id' }),
    })

    renderWithProviders(<PasskeyLoginButton onError={onError} />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(onError).toHaveBeenCalledWith('Failed to start passkey login')
    })
    expect(mockStartAuthentication).not.toHaveBeenCalled()
  })
})
