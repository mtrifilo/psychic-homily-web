import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AuthPage from './page'

// The Sign in tab's own form: what pressing Sign in does with invalid fields,
// and where a refused sign-in's alert leaves the viewer.

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/auth',
}))

vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return {
    useAuthContext: () =>
      deriveMockAuthSignals({ setUser: vi.fn(), authStatus: 'anonymous' }),
  }
})

const mockLoginMutate = vi.fn()
// The error the login mutation reports, as a refused sign-in leaves it.
let mockLoginError: Error | null = null
vi.mock('@/features/auth', () => ({
  useLogin: () => ({
    mutate: mockLoginMutate,
    // As the real mutation's reset does, clears the error it reports.
    reset: () => {
      mockLoginError = null
    },
    isPending: false,
    error: mockLoginError,
  }),
  useRegister: () => ({ mutate: vi.fn(), isPending: false, error: null as Error | null }),
  useSendMagicLink: () => ({ mutate: vi.fn(), isPending: false }),
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

function signInButton() {
  return screen.getByRole('button', { name: 'Sign in' })
}

describe('LoginForm submit', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockLoginMutate.mockReset()
    mockLoginError = null
    vi.mocked(Element.prototype.scrollIntoView).mockClear()
  })

  it('keeps Sign in enabled with empty fields', () => {
    renderWithProviders(<AuthPage />)

    expect(signInButton()).toBeEnabled()
  })

  it('focuses Email, the first invalid field, when every field is empty', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AuthPage />)

    await user.click(signInButton())

    await waitFor(() => {
      expect(screen.getByLabelText('Email')).toHaveFocus()
    })
    expect(screen.getByText('Please enter a valid email address')).toBeInTheDocument()
    expect(screen.getByText('Password is required')).toBeInTheDocument()
    const scroll = vi.mocked(Element.prototype.scrollIntoView).mock
    expect(scroll.contexts.at(-1)).toBe(screen.getByLabelText('Email'))
    expect(scroll.calls.at(-1)).toEqual([{ block: 'center' }])
    expect(signInButton()).toBeEnabled()
    expect(mockLoginMutate).not.toHaveBeenCalled()
  })

  it('focuses Password from the keyboard when it is the only invalid field', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AuthPage />)

    await user.type(screen.getByLabelText('Email'), 'user@example.com{Enter}')

    await waitFor(() => {
      expect(screen.getByLabelText('Password')).toHaveFocus()
    })
    expect(screen.getByText('Password is required')).toBeInTheDocument()
    expect(document.activeElement).not.toBe(document.body)
    expect(mockLoginMutate).not.toHaveBeenCalled()
  })

  it('submits valid fields once', async () => {
    const user = userEvent.setup()
    renderWithProviders(<AuthPage />)

    await user.type(screen.getByLabelText('Email'), 'user@example.com')
    await user.type(screen.getByLabelText('Password'), 'a-password{Enter}')

    await waitFor(() => {
      expect(mockLoginMutate).toHaveBeenCalledTimes(1)
    })
    expect(mockLoginMutate.mock.calls[0][0]).toEqual({
      email: 'user@example.com',
      password: 'a-password',
    })
  })

  // The alert announces itself, so focus stays on the password field; the
  // alert is scrolled to first and the field after it, so the field stays in
  // view when both do not fit.
  it('brings a refused sign-in\'s alert into view and leaves focus on Password', async () => {
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(<AuthPage />)
    await user.type(screen.getByLabelText('Email'), 'user@example.com')
    const password = screen.getByLabelText('Password')
    await user.type(password, 'wrong-password{Enter}')
    await waitFor(() => {
      expect(mockLoginMutate).toHaveBeenCalledTimes(1)
    })
    vi.mocked(Element.prototype.scrollIntoView).mockClear()

    mockLoginError = new Error('Invalid email or password')
    rerender(<AuthPage />)

    const alert = screen.getByText('Invalid email or password').closest('[role="alert"]')
    expect(alert).not.toBeNull()
    expect(vi.mocked(Element.prototype.scrollIntoView).mock.contexts).toEqual([
      alert,
      password,
    ])
    expect(password).toHaveFocus()
  })
})
