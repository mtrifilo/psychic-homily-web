import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SaveButton } from './SaveButton'

const mockToggle = vi.fn()
type MockUseSaveShowToggleValue = {
  isLoading: boolean
  toggle: typeof mockToggle
  error: Error | null
}
const mockUseSaveShowToggle = vi.fn<
  (..._args: unknown[]) => MockUseSaveShowToggleValue
>(() => ({
  isLoading: false,
  toggle: mockToggle,
  error: null,
}))

type MockUseShowSaveCountValue = {
  data?: { show_id: number; save_count: number; is_saved: boolean }
}
const mockUseShowSaveCount = vi.fn<
  (..._args: unknown[]) => MockUseShowSaveCountValue
>(() => ({ data: undefined }))

vi.mock('@/features/shows', () => ({
  useSaveShowToggle: (...args: unknown[]) => mockUseSaveShowToggle(...args),
  useShowSaveCount: (...args: unknown[]) => mockUseShowSaveCount(...args),
}))

const mockRouterPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockRouterPush }),
  usePathname: () => '/shows/1',
}))

type MockUseAuthContextValue = {
  user: { email: string } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockUseAuthContext = vi.fn<() => MockUseAuthContextValue>(() => ({
  isAuthenticated: true,
  user: { email: 'test@test.com' },
  isLoading: false,
  logout: vi.fn(),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

function anonymous() {
  mockUseAuthContext.mockReturnValue({
    isAuthenticated: false,
    user: null,
    isLoading: false,
    logout: vi.fn(),
  })
}

describe('SaveButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseAuthContext.mockReturnValue({
      isAuthenticated: true,
      user: { email: 'test@test.com' },
      isLoading: false,
      logout: vi.fn(),
    })
    mockUseSaveShowToggle.mockReturnValue({
      isLoading: false,
      toggle: mockToggle,
      error: null,
    })
    mockUseShowSaveCount.mockReturnValue({ data: undefined })
  })

  // ── Anonymous visitors
  //
  // The save COUNT is public, so the button must render logged-out (matching
  // FollowButton) rather than returning null. Clicking routes to sign-in.

  it('renders for anonymous visitors with a sign-in affordance', () => {
    anonymous()
    render(<SaveButton showId={1} />)
    expect(screen.getByRole('button')).toBeInTheDocument()
    expect(screen.getByLabelText('Sign in to save')).toBeInTheDocument()
  })

  it('shows the public save count to anonymous visitors', () => {
    anonymous()
    render(<SaveButton showId={1} saveData={{ save_count: 7, is_saved: false }} />)
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.getByLabelText('Sign in to save (7 saved)')).toBeInTheDocument()
  })

  it('redirects anonymous visitors to sign-in instead of toggling', async () => {
    const user = userEvent.setup()
    anonymous()
    render(<SaveButton showId={1} />)

    await user.click(screen.getByRole('button'))
    expect(mockRouterPush).toHaveBeenCalledWith('/auth?returnTo=%2Fshows%2F1')
    expect(mockToggle).not.toHaveBeenCalled()
  })

  // ── Count rendering

  it('renders the save count from saveData without fetching its own', () => {
    render(<SaveButton showId={1} saveData={{ save_count: 12, is_saved: false }} />)
    expect(screen.getByText('12')).toBeInTheDocument()
    // saveData supplied => the single-show query is disabled
    expect(mockUseShowSaveCount).toHaveBeenCalledWith(1, true, false)
  })

  it('fetches its own count when saveData is absent', () => {
    mockUseShowSaveCount.mockReturnValue({
      data: { show_id: 1, save_count: 4, is_saved: true },
    })
    render(<SaveButton showId={1} />)
    expect(mockUseShowSaveCount).toHaveBeenCalledWith(1, true, true)
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(screen.getByLabelText('Remove from My List (4 saved)')).toBeInTheDocument()
  })

  it('hides the count when zero', () => {
    render(<SaveButton showId={1} saveData={{ save_count: 0, is_saved: false }} />)
    expect(screen.queryByText('0')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Add to My List')).toBeInTheDocument()
  })

  // ── Saved / unsaved state

  it('renders save button when authenticated', () => {
    render(<SaveButton showId={1} />)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('has "Add to My List" aria-label and title when not saved', () => {
    render(<SaveButton showId={1} />)
    expect(screen.getByLabelText('Add to My List')).toBeInTheDocument()
    expect(screen.getByTitle('Add to My List')).toBeInTheDocument()
  })

  it('has "Remove from My List" aria-label and title when saved', () => {
    render(<SaveButton showId={1} saveData={{ save_count: 0, is_saved: true }} />)
    expect(screen.getByLabelText('Remove from My List')).toBeInTheDocument()
    expect(screen.getByTitle('Remove from My List')).toBeInTheDocument()
  })

  it('passes showId and the resolved isSaved to useSaveShowToggle', () => {
    render(<SaveButton showId={42} saveData={{ save_count: 2, is_saved: true }} />)
    expect(mockUseSaveShowToggle).toHaveBeenCalledWith(42, true)
  })

  it('defaults isSaved to false while the count query is still loading', () => {
    render(<SaveButton showId={42} />)
    expect(mockUseSaveShowToggle).toHaveBeenCalledWith(42, false)
  })

  // ── Interaction

  it('calls toggle on click', async () => {
    const user = userEvent.setup()
    render(<SaveButton showId={1} />)

    await user.click(screen.getByRole('button'))
    expect(mockToggle).toHaveBeenCalledOnce()
  })

  it('disables button while in-flight to prevent double-clicks', () => {
    mockUseSaveShowToggle.mockReturnValue({
      isLoading: true,
      toggle: mockToggle,
      error: null,
    })
    render(<SaveButton showId={1} />)
    // isLoading=true must disable the button so a user can't click twice
    // and fire two concurrent toggles against the same show.
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('stops event propagation on click', async () => {
    const user = userEvent.setup()
    const parentClick = vi.fn()
    render(
      <div onClick={parentClick}>
        <SaveButton showId={1} />
      </div>
    )

    await user.click(screen.getByRole('button'))
    expect(parentClick).not.toHaveBeenCalled()
  })

  it('shows error tooltip when toggle fails, keeping the un-saved state', async () => {
    const user = userEvent.setup()
    const toggleErr = new Error('Server is down')
    const toggleRejects = vi.fn(async () => {
      throw toggleErr
    })
    mockUseSaveShowToggle.mockReturnValue({
      isLoading: false,
      toggle: toggleRejects,
      error: toggleErr,
    })

    render(<SaveButton showId={1} />)
    await user.click(screen.getByRole('button'))

    // The hook rolls the cache back, so the button stays un-saved and the
    // failure surfaces in the tooltip.
    expect(await screen.findByText(/Failed to save show/)).toBeInTheDocument()
    expect(screen.getByLabelText('Add to My List')).toBeInTheDocument()
  })

  // ── showLabel variant

  it('shows "Save" label when showLabel is true and not saved', () => {
    render(<SaveButton showId={1} showLabel={true} />)
    expect(screen.getByText('Save')).toBeInTheDocument()
  })

  it('shows "Saved" label when showLabel is true and saved', () => {
    render(
      <SaveButton showId={1} showLabel={true} saveData={{ save_count: 1, is_saved: true }} />
    )
    expect(screen.getByText('Saved')).toBeInTheDocument()
  })

  it('does not show text label when showLabel is false', () => {
    render(<SaveButton showId={1} showLabel={false} />)
    expect(screen.queryByText('Save')).not.toBeInTheDocument()
    expect(screen.queryByText('Saved')).not.toBeInTheDocument()
  })
})
