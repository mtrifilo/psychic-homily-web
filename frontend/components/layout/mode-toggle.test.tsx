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

  it('clicking twice in light mode alternates dark → light (no idle state)', async () => {
    const user = userEvent.setup()
    // Use a stateful mock so the second click reads the new resolvedTheme.
    let currentTheme = 'light'
    const statefulSetTheme = vi.fn((t: string) => {
      currentTheme = t
      mockSetTheme(t)
    })
    mockUseTheme.mockImplementation(() => ({
      resolvedTheme: currentTheme,
      setTheme: statefulSetTheme,
    }))

    const { rerender } = render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenNthCalledWith(1, 'dark')

    // Re-render so the next useTheme() call reads the updated currentTheme.
    rerender(<ModeToggle />)
    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenNthCalledWith(2, 'light')
  })

  it('only renders ONE setTheme call per click (no double-bind regression)', async () => {
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(mockSetTheme).toHaveBeenCalledTimes(1)
  })

  it('handles undefined resolvedTheme gracefully (initial hydration)', async () => {
    // next-themes returns undefined briefly during SSR hydration. The
    // toggle must default sensibly (treat undefined !== 'dark' → set dark)
    // rather than throw.
    mockUseTheme.mockReturnValue({
      resolvedTheme: undefined as unknown as string,
      setTheme: mockSetTheme,
    })
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    // resolvedTheme === undefined → 'dark' branch in toggleTheme
    expect(mockSetTheme).toHaveBeenCalledWith('dark')
  })

  it('includes sr-only label so screen readers announce purpose', () => {
    render(<ModeToggle />)
    const srLabel = screen.getByText('Toggle theme')
    expect(srLabel.tagName).toBe('SPAN')
    expect(srLabel.className).toContain('sr-only')
  })
})
