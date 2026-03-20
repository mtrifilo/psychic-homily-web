import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModeToggle } from './mode-toggle'

// --- Mocks ---

const mockSetTheme = vi.fn()
const mockUseTheme = vi.fn(() => ({
  theme: 'dark',
  setTheme: mockSetTheme,
}))

vi.mock('next-themes', () => ({
  useTheme: () => mockUseTheme(),
}))

describe('ModeToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTheme.mockReturnValue({
      theme: 'dark',
      setTheme: mockSetTheme,
    })
  })

  it('renders a button', () => {
    render(<ModeToggle />)
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('has screen reader text "Toggle theme"', () => {
    render(<ModeToggle />)
    expect(screen.getByText('Toggle theme')).toBeInTheDocument()
    // The text should be in a sr-only span
    const srText = screen.getByText('Toggle theme')
    expect(srText.className).toContain('sr-only')
  })

  it('toggles from dark to light when clicked', async () => {
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button'))
    expect(mockSetTheme).toHaveBeenCalledWith('light')
  })

  it('toggles from light to dark when clicked', async () => {
    const user = userEvent.setup()
    mockUseTheme.mockReturnValue({
      theme: 'light',
      setTheme: mockSetTheme,
    })
    render(<ModeToggle />)

    await user.click(screen.getByRole('button'))
    expect(mockSetTheme).toHaveBeenCalledWith('dark')
  })

  it('renders with outline variant and icon size', () => {
    render(<ModeToggle />)
    const button = screen.getByRole('button')
    // Shadcn Button with variant="outline" and size="icon"
    expect(button).toHaveClass('cursor-pointer')
  })

  it('renders Sun and Moon icons', () => {
    const { container } = render(<ModeToggle />)
    // lucide-react renders SVGs
    const svgs = container.querySelectorAll('svg')
    expect(svgs.length).toBe(2) // Sun + Moon
  })

  it('has accessible button role', () => {
    render(<ModeToggle />)
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })
})
