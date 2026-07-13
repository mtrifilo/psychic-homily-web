import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReleaseSaveButton } from './ReleaseSaveButton'

const mockToggle = vi.fn()
const mockPush = vi.fn()
const mockUseAuthContext = vi.fn(() => ({ isAuthenticated: true }))
const mockUseReleaseSaveCount = vi.fn<
  (...args: unknown[]) => { data: undefined; isLoading: boolean }
>(() => ({ data: undefined, isLoading: false }))
const mockUseReleaseSaveToggle = vi.fn<
  (...args: unknown[]) => {
    toggle: typeof mockToggle
    isLoading: boolean
    error: Error | null
  }
>(() => ({ toggle: mockToggle, isLoading: false, error: null }))

vi.mock('next/navigation', () => ({
  usePathname: () => '/releases/the-record',
  useRouter: () => ({ push: mockPush }),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))
vi.mock('@/features/releases', () => ({
  useReleaseSaveCount: (...args: unknown[]) => mockUseReleaseSaveCount(...args),
  useReleaseSaveToggle: (...args: unknown[]) =>
    mockUseReleaseSaveToggle(...args),
}))

describe('ReleaseSaveButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/releases/the-record')
    mockUseAuthContext.mockReturnValue({ isAuthenticated: true })
    mockUseReleaseSaveCount.mockReturnValue({
      data: undefined,
      isLoading: false,
    })
    mockUseReleaseSaveToggle.mockReturnValue({
      toggle: mockToggle,
      isLoading: false,
      error: null,
    })
  })

  it('renders the approved bracket action and toggles a release', async () => {
    const user = userEvent.setup()
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: false }}
        variant="bracket"
      />
    )

    await user.click(screen.getByRole('button', { name: 'Save release' }))
    expect(mockUseReleaseSaveToggle).toHaveBeenCalledWith(17, false, undefined)
    expect(mockToggle).toHaveBeenCalledOnce()
  })

  it('renders Saved for an existing save', () => {
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: true }}
        variant="bracket"
      />
    )
    expect(
      screen.getByRole('button', { name: 'Remove saved release' })
    ).toHaveTextContent('[Saved]')
  })

  it('supports the dense Library remove label and accessible release name', () => {
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: true }}
        variant="bracket"
        bracketLabel="× remove"
        ariaLabel="Remove Clarity from saved releases"
      />
    )

    expect(
      screen.getByRole('button', {
        name: 'Remove Clarity from saved releases',
      })
    ).toHaveTextContent('[× remove]')
  })

  it('sends anonymous users to auth with the current release as returnTo', async () => {
    const user = userEvent.setup()
    mockUseAuthContext.mockReturnValue({ isAuthenticated: false })
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: false }}
      />
    )

    await user.click(screen.getByRole('button'))
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Freleases%2Fthe-record'
    )
    expect(mockToggle).not.toHaveBeenCalled()
  })

  it('preserves active query state in the sign-in return path', async () => {
    const user = userEvent.setup()
    window.history.replaceState({}, '', '/releases/the-record?window=all_time')
    mockUseAuthContext.mockReturnValue({ isAuthenticated: false })
    render(
      <ReleaseSaveButton
        releaseId={17}
        saveData={{ save_count: 4, is_saved: false }}
      />
    )

    await user.click(screen.getByRole('button'))
    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Freleases%2Fthe-record%3Fwindow%3Dall_time'
    )
  })
})
