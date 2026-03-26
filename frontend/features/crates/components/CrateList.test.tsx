import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CrateList } from './CrateList'
import type { Crate } from '../types'

// Mock AuthContext
const mockAuthContext = vi.fn(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock crate hooks
const mockUseCrates = vi.fn()
const mockCreateMutate = vi.fn()

vi.mock('../hooks', () => ({
  useCrates: () => mockUseCrates(),
  useCreateCrate: () => ({
    mutate: mockCreateMutate,
    isPending: false,
    error: null,
  }),
}))

// Mock child components
vi.mock('./CrateCard', () => ({
  CrateCard: ({ crate }: { crate: Crate }) => (
    <article data-testid={`crate-card-${crate.id}`}>{crate.title}</article>
  ),
}))

vi.mock('@/components/shared', () => ({
  LoadingSpinner: () => <div data-testid="loading-spinner">Loading...</div>,
}))

vi.mock('@/components/ui/button', () => ({
  Button: ({ children, onClick, disabled, ...props }: {
    children: React.ReactNode
    onClick?: () => void
    disabled?: boolean
    [key: string]: unknown
  }) => (
    <button onClick={onClick} disabled={disabled} type={props.type as string}>{children}</button>
  ),
}))

vi.mock('@/components/ui/input', () => ({
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
}))

vi.mock('@/components/ui/textarea', () => ({
  Textarea: (props: React.TextareaHTMLAttributes<HTMLTextAreaElement>) => <textarea {...props} />,
}))

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) => (
    <div data-testid="dialog" data-open={open}>{children}</div>
  ),
  DialogContent: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="dialog-content">{children}</div>
  ),
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogTrigger: ({ children, asChild }: { children: React.ReactNode; asChild?: boolean }) => (
    <>{children}</>
  ),
}))

function makeCrate(overrides: Partial<Crate> = {}): Crate {
  return {
    id: 1,
    title: 'Test Crate',
    slug: 'test-crate',
    description: 'A test crate',
    creator_id: 1,
    creator_name: 'testuser',
    collaborative: false,
    is_public: true,
    is_featured: false,
    item_count: 5,
    subscriber_count: 3,
    contributor_count: 1,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('CrateList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  describe('loading state', () => {
    it('shows loading spinner when loading and no data', () => {
      mockUseCrates.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByTestId('loading-spinner')).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('shows error message when fetch fails', () => {
      mockUseCrates.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Network error'),
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByText('Failed to load crates. Please try again later.')).toBeInTheDocument()
    })

    it('shows retry button on error', () => {
      const mockRefetch = vi.fn()
      mockUseCrates.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      render(<CrateList />)
      expect(screen.getByText('Retry')).toBeInTheDocument()
    })

    it('calls refetch when retry clicked', async () => {
      const user = userEvent.setup()
      const mockRefetch = vi.fn()
      mockUseCrates.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error('Network error'),
        refetch: mockRefetch,
      })
      render(<CrateList />)
      await user.click(screen.getByText('Retry'))
      expect(mockRefetch).toHaveBeenCalled()
    })
  })

  describe('empty state', () => {
    it('shows empty message when no crates', () => {
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByText('No public crates yet.')).toBeInTheDocument()
    })

    it('shows encouragement for authenticated user when empty', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByText('Be the first to create one!')).toBeInTheDocument()
    })

    it('does not show encouragement for unauthenticated user', () => {
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.queryByText('Be the first to create one!')).not.toBeInTheDocument()
    })
  })

  describe('with crates', () => {
    it('renders crate cards', () => {
      mockUseCrates.mockReturnValue({
        data: {
          crates: [
            makeCrate({ id: 1, title: 'Crate One' }),
            makeCrate({ id: 2, title: 'Crate Two' }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByTestId('crate-card-1')).toBeInTheDocument()
      expect(screen.getByTestId('crate-card-2')).toBeInTheDocument()
    })

    it('separates featured and regular crates', () => {
      mockUseCrates.mockReturnValue({
        data: {
          crates: [
            makeCrate({ id: 1, title: 'Featured One', is_featured: true }),
            makeCrate({ id: 2, title: 'Regular One', is_featured: false }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByText('Featured')).toBeInTheDocument()
      expect(screen.getByText('All Crates')).toBeInTheDocument()
    })

    it('does not show Featured heading when no featured crates', () => {
      mockUseCrates.mockReturnValue({
        data: {
          crates: [
            makeCrate({ id: 1, title: 'Regular One', is_featured: false }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.queryByText('Featured')).not.toBeInTheDocument()
      expect(screen.queryByText('All Crates')).not.toBeInTheDocument()
    })

    it('does not show All Crates heading when only featured', () => {
      mockUseCrates.mockReturnValue({
        data: {
          crates: [
            makeCrate({ id: 1, title: 'Featured One', is_featured: true }),
          ],
        },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.getByText('Featured')).toBeInTheDocument()
      // The "All Crates" heading only shows when both featured and regular exist
      expect(screen.queryByText('All Crates')).not.toBeInTheDocument()
    })
  })

  describe('create crate button', () => {
    it('shows create button for authenticated user', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      // Button and dialog title both render "Create Crate"; verify button exists
      const matches = screen.getAllByText('Create Crate')
      const button = matches.find(el => el.closest('button'))
      expect(button).toBeTruthy()
    })

    it('does not show create button for unauthenticated user', () => {
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      expect(screen.queryByText('Create Crate')).not.toBeInTheDocument()
    })

    it('shows create dialog with form', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      // Dialog content renders (since we mock Dialog to always render children)
      // "Create Crate" appears as both button text and dialog title
      expect(screen.getAllByText('Create Crate').length).toBeGreaterThanOrEqual(2)
      expect(screen.getByLabelText('Title')).toBeInTheDocument()
      expect(screen.getByLabelText('Description (optional)')).toBeInTheDocument()
    })

    it('renders create form with Public checkbox checked by default', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      const publicCheckbox = screen.getByLabelText('Public') as HTMLInputElement
      expect(publicCheckbox.checked).toBe(true)
    })

    it('renders create form with Collaborative checkbox unchecked by default', () => {
      mockAuthContext.mockReturnValue({
        user: { id: '1' },
        isAuthenticated: true,
        isLoading: false,
        logout: vi.fn(),
      })
      mockUseCrates.mockReturnValue({
        data: { crates: [] },
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      })
      render(<CrateList />)
      const collabCheckbox = screen.getByLabelText('Collaborative') as HTMLInputElement
      expect(collabCheckbox.checked).toBe(false)
    })
  })
})
