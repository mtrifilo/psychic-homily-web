import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ModeToggle, useThemeToggle } from './mode-toggle'

const mockSetTheme = vi.fn()
const mockUseTheme = vi.fn(() => ({
  theme: 'light',
  resolvedTheme: 'light',
  setTheme: mockSetTheme,
}))

vi.mock('next-themes', () => ({
  useTheme: () => mockUseTheme(),
}))

beforeEach(() => {
  vi.clearAllMocks()
  mockUseTheme.mockReturnValue({
    theme: 'light',
    resolvedTheme: 'light',
    setTheme: mockSetTheme,
  })
})

// PSY-1818: useThemeToggle is the ONE theme-flip implementation. Every flip
// rule is pinned here, once — the top bar, the mobile Browse sheet and the hero
// lab each used to carry their own copy of the expression and, for the
// resolvedTheme rule, their own copy of the regression test.
describe('useThemeToggle', () => {
  it('reports the light theme and labels the action "Dark mode"', () => {
    const { result } = renderHook(() => useThemeToggle())
    expect(result.current.isDark).toBe(false)
    expect(result.current.label).toBe('Dark mode')
  })

  it('reports the dark theme and labels the ACTION, not the state', () => {
    mockUseTheme.mockReturnValue({
      theme: 'dark',
      resolvedTheme: 'dark',
      setTheme: mockSetTheme,
    })
    const { result } = renderHook(() => useThemeToggle())
    expect(result.current.isDark).toBe(true)
    // "Light mode" = what the click does, not where the user is now.
    expect(result.current.label).toBe('Light mode')
  })

  it('flips the VISIBLE theme under theme="system" — resolvedTheme, not theme', () => {
    // The regression this hook exists to hold in one place: with
    // theme === 'system' on a dark device, a `theme === 'dark'` check sets an
    // explicit 'dark' — the first click appears to do nothing. resolvedTheme
    // reports what the user is actually looking at. `theme` is set to 'system'
    // here on purpose: without it the test would pass against a hook that keys
    // off `theme`, which is exactly the bug.
    mockUseTheme.mockReturnValue({
      theme: 'system',
      resolvedTheme: 'dark',
      setTheme: mockSetTheme,
    })
    const { result } = renderHook(() => useThemeToggle())
    act(() => result.current.toggle())
    expect(mockSetTheme).toHaveBeenCalledTimes(1)
    expect(mockSetTheme).toHaveBeenCalledWith('light')
  })

  it('treats an undefined resolvedTheme (pre-hydration) as not-dark', () => {
    // next-themes reports undefined until it has read storage; the toggle must
    // default sensibly rather than throw.
    mockUseTheme.mockReturnValue({
      theme: 'system',
      resolvedTheme: undefined as unknown as string,
      setTheme: mockSetTheme,
    })
    const { result } = renderHook(() => useThemeToggle())
    expect(result.current.isDark).toBe(false)
    expect(result.current.label).toBe('Dark mode')
    act(() => result.current.toggle())
    expect(mockSetTheme).toHaveBeenCalledWith('dark')
  })

  it('alternates on repeated clicks (no idle state)', () => {
    let currentTheme = 'light'
    const statefulSetTheme = vi.fn((t: string) => {
      currentTheme = t
      mockSetTheme(t)
    })
    mockUseTheme.mockImplementation(() => ({
      theme: currentTheme,
      resolvedTheme: currentTheme,
      setTheme: statefulSetTheme,
    }))

    const { result, rerender } = renderHook(() => useThemeToggle())

    act(() => result.current.toggle())
    expect(mockSetTheme).toHaveBeenNthCalledWith(1, 'dark')

    // Re-render so the hook reads the updated resolvedTheme.
    rerender()
    act(() => result.current.toggle())
    expect(mockSetTheme).toHaveBeenNthCalledWith(2, 'light')
  })
})

// ModeToggle's own contract is the button chrome around that hook. The flip
// rules above are not re-asserted through the component.
describe('ModeToggle', () => {
  it('renders the toggle button', () => {
    render(<ModeToggle />)
    expect(screen.getByRole('button', { name: 'Toggle theme' })).toBeInTheDocument()
  })

  it('flips the theme through useThemeToggle, once per click', async () => {
    const user = userEvent.setup()
    render(<ModeToggle />)

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    // Once, not twice: guards against re-binding the handler on a wrapper.
    expect(mockSetTheme).toHaveBeenCalledTimes(1)
    expect(mockSetTheme).toHaveBeenCalledWith('dark')
  })

  it('includes sr-only label so screen readers announce purpose', () => {
    render(<ModeToggle />)
    const srLabel = screen.getByText('Toggle theme')
    expect(srLabel.tagName).toBe('SPAN')
    expect(srLabel.className).toContain('sr-only')
  })
})
