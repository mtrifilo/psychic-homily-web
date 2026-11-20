import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AuthPage from './page'

// "Email me a sign-in link instead" asks for a link that returns to the
// destination the page would send a password sign-in to: the arrival's
// returnTo once sanitizeReturnTo has checked it.

let mockSearchParams = new URLSearchParams()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => '/auth',
}))

vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ setUser: vi.fn(), authStatus: 'anonymous' }),
  }
})

const mockSendMagicLink = vi.fn()

vi.mock('@/features/auth', () => ({
  useLogin: () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false, error: null }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useRequestPasswordReset: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useSendMagicLink: () => ({ mutate: mockSendMagicLink, isPending: false }),
  useSendVerificationEmail: () => ({
    mutate: vi.fn(),
    isPending: false,
    isSuccess: false,
    isError: false,
    error: null as Error | null,
  }),
}))

vi.mock('@/app/auth/_components/passkey-login', () => ({
  PasskeyLoginButton: (): null => null,
}))

vi.mock('@/app/auth/_components/google-oauth-button', () => ({
  GoogleOAuthButton: (): null => null,
}))

async function askForSignInLink(query: string) {
  mockSearchParams = new URLSearchParams(query)
  const user = userEvent.setup()
  renderWithProviders(<AuthPage />)
  await user.type(screen.getByLabelText('Email'), 'owner@example.com')
  await user.click(screen.getByRole('button', { name: 'Email me a sign-in link instead' }))
}

describe('the sign-in link destination', () => {
  beforeEach(() => {
    mockSendMagicLink.mockReset()
  })

  it.each([
    ['a deep page', 'returnTo=%2Flibrary', '/library'],
    ['a page with a query and fragment', 'returnTo=%2Fshows%3Fcity%3Dphoenix%23tonight', '/shows?city=phoenix#tonight'],
  ])('asks for a link back to %s', async (_name, query, expected) => {
    await askForSignInLink(query)

    expect(mockSendMagicLink).toHaveBeenCalledTimes(1)
    expect(mockSendMagicLink.mock.calls[0][0]).toEqual({ email: 'owner@example.com', returnTo: expected })
  })

  it.each([
    ['no destination', ''],
    ['a protocol-relative destination', 'returnTo=%2F%2Fevil.example'],
    ['an off-site destination', 'returnTo=https%3A%2F%2Fevil.example%2Flibrary'],
    ['a backslash destination', 'returnTo=%2F%5Cevil.example'],
    ['the sign-in page itself', 'returnTo=%2Fauth%2Fmagic-link'],
  ])('asks for a link back to the root for %s', async (_name, query) => {
    await askForSignInLink(query)

    expect(mockSendMagicLink).toHaveBeenCalledTimes(1)
    expect(mockSendMagicLink.mock.calls[0][0]).toEqual({ email: 'owner@example.com', returnTo: '/' })
  })
})
