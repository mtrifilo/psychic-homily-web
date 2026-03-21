import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModeToggle } from './mode-toggle'

const mockSetTheme = vi.fn()
const mockUseTheme = vi.fn(() => ({
  resolvedTheme: 'light',
  setTheme: mockSetTheme,
}))

vi.mock('next-themes', () => ({
  useTheme: () => mockUseTheme(),
}))

describe('ModeToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseTheme.mockReturnValue({
      resolvedTheme: 'light',
      setTheme: mockSetTheme,
    })
  })

  it('renders the toggle button', () => {
    render(<ModeToggle />)
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })

  it('toggles from light to dark when resolvedTheme is light', async () => {
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenCalledWith('dark')
  })

  it('toggles from dark to light when resolvedTheme is dark', async () => {
    mockUseTheme.mockReturnValue({
      resolvedTheme: 'dark',
      setTheme: mockSetTheme,
    })
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenCalledWith('light')
  })

  it('uses resolvedTheme not theme — when system prefers dark, toggle sets light', async () => {
    // This is the bug fix: resolvedTheme reflects the actual system preference,
    // while theme would be 'system' and incorrectly treated as light.
    mockUseTheme.mockReturnValue({
      resolvedTheme: 'dark', // system resolved to dark
      setTheme: mockSetTheme,
    })
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenCalledWith('light')
    expect(mockSetTheme).not.toHaveBeenCalledWith('dark')
  })
})
