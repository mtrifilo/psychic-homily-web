import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TopBar } from './TopBar'

vi.mock('next/navigation', () => ({
  usePathname: () => '/',
}))

vi.mock('next/image', () => ({
  default: (props: Record<string, unknown>) => {
    const { priority, ...rest } = props
    return <img {...rest} data-priority={priority ? 'true' : undefined} />
  },
}))

const mockAuthContext = vi.fn(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

vi.mock('next-themes', () => ({
  useTheme: () => ({ theme: 'dark', setTheme: vi.fn() }),
}))

describe('TopBar', () => {
  const onMobileOpenChange = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  it('renders logo image', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByAltText('Psychic Homily Logo')).toBeInTheDocument()
  })

  it('renders site name', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByText('Psychic Homily')).toBeInTheDocument()
  })

  it('renders search placeholder button', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByText('Search...')).toBeInTheDocument()
    expect(screen.getByText('⌘K')).toBeInTheDocument()
  })

  it('shows login link when unauthenticated', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByText('login / sign-up')).toBeInTheDocument()
  })

  it('shows user avatar button when authenticated', () => {
    mockAuthContext.mockReturnValue({
      user: { email: 'test@test.com', first_name: 'John', last_name: 'Doe', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByRole('button', { name: 'User menu' })).toBeInTheDocument()
    expect(screen.getByText('JD')).toBeInTheDocument()
  })

  it('shows loading spinner while auth is loading', () => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: true,
      logout: vi.fn(),
    })
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.queryByText('login / sign-up')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'User menu' })).not.toBeInTheDocument()
  })

  it('renders hamburger menu button', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByRole('button', { name: 'Open menu' })).toBeInTheDocument()
  })

  it('renders theme toggle button', () => {
    render(<TopBar mobileOpen={false} onMobileOpenChange={onMobileOpenChange} />)
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })
})
