import { StrictMode } from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { BackupAuthPrompt } from './backup-auth-prompt'

const OAUTH_BACKEND = 'https://oauth-backend.test'

vi.mock('@/lib/api-base', () => ({
  OAUTH_BACKEND_URL: 'https://oauth-backend.test',
}))

const mockStartLinkMutateAsync = vi.fn()
vi.mock('@/features/auth/hooks/useAuth', () => ({
  useStartOAuthLink: () => ({ mutateAsync: mockStartLinkMutateAsync }),
}))

const mockCaptureException = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureException: (...args: unknown[]) => mockCaptureException(...args),
}))

const PROMPT = { name: 'Add a backup sign-in method' }
const CONNECT = { name: /Connect Google account/ }

const originalLocation = window.location
const navigate = vi.fn()

function renderPrompt() {
  const onOpenChange = vi.fn()
  const user = userEvent.setup()
  renderWithProviders(<BackupAuthPrompt open onOpenChange={onOpenChange} />)
  const prompt = screen.getByRole('dialog', PROMPT)
  return { user, prompt, onOpenChange }
}

describe('BackupAuthPrompt', () => {
  beforeEach(() => {
    mockStartLinkMutateAsync.mockReset()
    mockCaptureException.mockReset()
    navigate.mockReset()
    Object.defineProperty(window, 'location', {
      value: { ...originalLocation, href: '' },
      writable: true,
      configurable: true,
    })
    Object.defineProperty(window.location, 'href', {
      set: navigate,
      configurable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
  })

  it('starts the authenticated link route with a freshly minted token', async () => {
    // Characters a token must not carry raw in a query string, so the test
    // also pins the encoding.
    mockStartLinkMutateAsync.mockResolvedValue({ token: 'tok+en/=&x' })
    const { user, prompt } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))

    expect(mockStartLinkMutateAsync).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        `${OAUTH_BACKEND}/auth/link/google?t=${encodeURIComponent('tok+en/=&x')}`
      )
    })
    expect(navigate).toHaveBeenCalledTimes(1)
  })

  it('does not navigate until the token exists, and holds the button meanwhile', async () => {
    let resolveMint: (value: { token: string }) => void = () => {}
    mockStartLinkMutateAsync.mockReturnValue(
      new Promise(resolve => {
        resolveMint = resolve
      })
    )
    const { user, prompt } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))

    expect(navigate).not.toHaveBeenCalled()
    expect(within(prompt).getByRole('button', CONNECT)).toBeDisabled()

    resolveMint({ token: 'later-token' })

    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        `${OAUTH_BACKEND}/auth/link/google?t=later-token`
      )
    })
  })

  it.each([
    ['skipped', "I'll do this later"],
    ['closed with its close button', 'Close'],
  ])('does not navigate when the prompt is %s while the token is pending', async (_, buttonName) => {
    let resolveMint: (value: { token: string }) => void = () => {}
    mockStartLinkMutateAsync.mockReturnValue(
      new Promise(resolve => {
        resolveMint = resolve
      })
    )
    const { user, prompt, onOpenChange } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))
    await user.click(within(prompt).getByRole('button', { name: buttonName }))
    expect(onOpenChange).toHaveBeenCalledWith(false)

    resolveMint({ token: 'late-token' })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(navigate).not.toHaveBeenCalled()
    // The parent here keeps the prompt mounted, which is what a page shown
    // again looks like: the button has to be usable, not left spinning.
    await waitFor(() => {
      expect(within(prompt).getByRole('button', CONNECT)).toBeEnabled()
    })
  })

  it('connects after the prompt is closed and opened again without remounting', async () => {
    mockStartLinkMutateAsync.mockResolvedValue({ token: 'second-try' })
    const onOpenChange = vi.fn()
    const user = userEvent.setup()
    const { rerender } = renderWithProviders(
      <BackupAuthPrompt open onOpenChange={onOpenChange} />
    )

    await user.click(
      within(screen.getByRole('dialog', PROMPT)).getByRole('button', {
        name: "I'll do this later",
      })
    )
    rerender(<BackupAuthPrompt open={false} onOpenChange={onOpenChange} />)
    rerender(<BackupAuthPrompt open onOpenChange={onOpenChange} />)

    await user.click(
      within(screen.getByRole('dialog', PROMPT)).getByRole('button', CONNECT)
    )

    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        `${OAUTH_BACKEND}/auth/link/google?t=second-try`
      )
    })
  })

  it('connects under StrictMode, whose double mount runs the unmount cleanup', async () => {
    mockStartLinkMutateAsync.mockResolvedValue({ token: 'strict-token' })
    const user = userEvent.setup()
    // Plain render: the shared provider wrapper does not reproduce the
    // StrictMode effect double-invoke this test is about.
    render(
      <StrictMode>
        <BackupAuthPrompt open onOpenChange={vi.fn()} />
      </StrictMode>
    )

    await user.click(
      within(screen.getByRole('dialog', PROMPT)).getByRole('button', CONNECT)
    )

    await waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        `${OAUTH_BACKEND}/auth/link/google?t=strict-token`
      )
    })
  })

  it('does not navigate when the prompt is closed with Escape while the token is pending', async () => {
    let resolveMint: (value: { token: string }) => void = () => {}
    mockStartLinkMutateAsync.mockReturnValue(
      new Promise(resolve => {
        resolveMint = resolve
      })
    )
    const { user, prompt, onOpenChange } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))
    await user.keyboard('{Escape}')
    expect(onOpenChange).toHaveBeenCalledWith(false)

    resolveMint({ token: 'late-token' })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(navigate).not.toHaveBeenCalled()
  })

  it('does not navigate when the prompt unmounts while the token is pending', async () => {
    let resolveMint: (value: { token: string }) => void = () => {}
    mockStartLinkMutateAsync.mockReturnValue(
      new Promise(resolve => {
        resolveMint = resolve
      })
    )
    const user = userEvent.setup()
    const { unmount } = renderWithProviders(
      <BackupAuthPrompt open onOpenChange={vi.fn()} />
    )

    await user.click(
      within(screen.getByRole('dialog', PROMPT)).getByRole('button', CONNECT)
    )
    unmount()

    resolveMint({ token: 'late-token' })
    await new Promise(resolve => setTimeout(resolve, 0))

    expect(navigate).not.toHaveBeenCalled()
  })

  it('stays open with the button back when the token cannot be minted', async () => {
    const failure = new Error('mint refused')
    mockStartLinkMutateAsync.mockRejectedValue(failure)
    const { user, prompt, onOpenChange } = renderPrompt()

    await user.click(within(prompt).getByRole('button', CONNECT))

    await waitFor(() => {
      expect(within(prompt).getByRole('button', CONNECT)).toBeEnabled()
    })
    expect(navigate).not.toHaveBeenCalled()
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', PROMPT)).toBeInTheDocument()
    expect(mockCaptureException).toHaveBeenCalledWith(
      failure,
      expect.objectContaining({ tags: { service: 'backup-auth-prompt' } })
    )
  })

  it('closes through onOpenChange when skipped, without starting a link', async () => {
    const { user, prompt, onOpenChange } = renderPrompt()

    await user.click(
      within(prompt).getByRole('button', { name: "I'll do this later" })
    )

    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(mockStartLinkMutateAsync).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()
  })
})
