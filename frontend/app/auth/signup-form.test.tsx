import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import AuthPage from './page'

// --- Mocks ---

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  useSearchParams: () => new URLSearchParams(),
}))

const mockRegisterMutate = vi.fn()
vi.mock('@/lib/hooks/useAuth', () => ({
  useRegister: () => ({
    mutate: mockRegisterMutate,
    isPending: false,
    error: null,
  }),
  useLogin: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
  useSendMagicLink: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    setUser: vi.fn(),
    isAuthenticated: false,
    isLoading: false,
  }),
}))

vi.mock('@/components/auth/passkey-login', () => ({
  PasskeyLoginButton: () => null,
}))

vi.mock('@/components/auth/passkey-signup', () => ({
  PasskeySignupButton: () => null,
}))

vi.mock('@/components/auth/google-oauth-button', () => ({
  GoogleOAuthButton: () => null,
}))

// --- Helpers ---

async function renderSignupForm() {
  const user = userEvent.setup()
  renderWithProviders(<AuthPage />)

  // Switch to the signup tab (Radix unmounts inactive tab content)
  await user.click(screen.getByRole('tab', { name: 'Create account' }))

  return { user }
}

// --- Tests ---

describe('SignupForm deferred validation', () => {
  beforeEach(() => {
    mockPush.mockReset()
    mockRegisterMutate.mockReset()
  })

  it('renders form fields without validation errors initially', async () => {
    await renderSignupForm()

    expect(screen.queryAllByRole('alert')).toHaveLength(0)
    expect(screen.getByLabelText('Email')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('checkbox')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled()
  })

  it('does not show errors while typing invalid input before submit', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'bur')
    await user.type(screen.getByLabelText('Password'), 'ab')

    expect(screen.queryAllByRole('alert')).toHaveLength(0)
  })

  it('shows validation errors on submit with empty fields', async () => {
    const { user } = await renderSignupForm()

    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      // Email + password + terms = 3 error alerts
      expect(screen.getAllByRole('alert')).toHaveLength(3)
    })
    expect(screen.getByText(/Please enter a valid email address/)).toBeInTheDocument()
    expect(screen.getByText(/Password must be at least 12 characters/)).toBeInTheDocument()
    expect(screen.getByText(/You must agree to the Terms of Service/)).toBeInTheDocument()
  })

  it('shows only email error when other fields are valid', async () => {
    const { user } = await renderSignupForm()

    // Leave email empty, fill password and accept terms
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(/Please enter a valid email address/)).toBeInTheDocument()
    })
    // Only email error — no terms or password errors
    expect(screen.queryByText(/You must agree to the Terms of Service/)).not.toBeInTheDocument()
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('shows password error text and aria-invalid on submit with short password', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'short')
    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByLabelText('Password')).toHaveAttribute('aria-invalid', 'true')
    })
    // Password error is now rendered as a visible alert
    expect(screen.getByText(/Password must be at least 12 characters/)).toBeInTheDocument()
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('shows terms error on submit without checking terms', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(/You must agree to the Terms of Service/)).toBeInTheDocument()
    })
  })

  it('clears errors in real-time after failed submit', async () => {
    const { user } = await renderSignupForm()

    // Submit empty form to trigger errors
    await user.click(screen.getByRole('button', { name: 'Create account' }))
    await waitFor(() => {
      expect(screen.getAllByRole('alert')).toHaveLength(3)
    })

    // Type valid email → email error clears
    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await waitFor(() => {
      expect(screen.queryByText(/Please enter a valid email address/)).not.toBeInTheDocument()
    })

    // Type valid password → password error clears
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await waitFor(() => {
      expect(screen.queryByText(/Password must be at least 12 characters/)).not.toBeInTheDocument()
    })

    // Check terms → terms error clears
    await user.click(screen.getByRole('checkbox'))
    await waitFor(() => {
      expect(screen.queryByText(/You must agree to the Terms of Service/)).not.toBeInTheDocument()
    })
  })

  it('calls register mutation on valid submit', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'test@example.com')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(mockRegisterMutate).toHaveBeenCalledWith(
        { email: 'test@example.com', password: 'validPassword123!' },
        expect.any(Object),
      )
    })
  })

  it('shows email error on submit with syntactically invalid email', async () => {
    const { user } = await renderSignupForm()

    await user.type(screen.getByLabelText('Email'), 'not-an-email')
    await user.type(screen.getByLabelText('Password'), 'validPassword123!')
    await user.click(screen.getByRole('checkbox'))
    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getByText(/Please enter a valid email address/)).toBeInTheDocument()
    })
    expect(mockRegisterMutate).not.toHaveBeenCalled()
  })

  it('does not show duplicate error messages', async () => {
    const { user } = await renderSignupForm()

    await user.click(screen.getByRole('button', { name: 'Create account' }))

    await waitFor(() => {
      expect(screen.getAllByRole('alert').length).toBeGreaterThanOrEqual(1)
    })

    // Find the email error alert and verify it has no duplicated text
    const emailError = screen.getByText('Please enter a valid email address')
    expect(emailError.textContent).toBe('Please enter a valid email address')
  })
})
