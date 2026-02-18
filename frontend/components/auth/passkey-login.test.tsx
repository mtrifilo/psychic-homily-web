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
        options: { challenge: 'begin-challenge' },
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

    renderWithProviders(<PasskeyLoginButton returnTo="/collection" />)

    await user.click(screen.getByRole('button', { name: /sign in with passkey/i }))

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith('/collection')
    })
  })
})
