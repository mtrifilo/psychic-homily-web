import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useTheme } from 'next-themes'
import { ThemeProvider } from './theme-provider'

// ThemeProvider is a thin pass-through to next-themes' NextThemesProvider.
// We exercise it END-TO-END via the real useTheme() hook so a regression
// in wiring (wrong prop forwarding, missing context, broken default theme)
// surfaces immediately. Mocking next-themes here would defeat the purpose.

function ThemeConsumer() {
  const { theme, resolvedTheme, setTheme, themes } = useTheme()
  return (
    <div>
      <div data-testid="theme">{theme ?? 'undefined'}</div>
      <div data-testid="resolved-theme">{resolvedTheme ?? 'undefined'}</div>
      <div data-testid="themes">{(themes ?? []).join(',')}</div>
      <button onClick={() => setTheme('dark')}>dark</button>
      <button onClick={() => setTheme('light')}>light</button>
      <button onClick={() => setTheme('system')}>system</button>
    </div>
  )
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    // next-themes persists the picked theme in localStorage between mounts.
    // Clear so tests start from a known default-system state.
    localStorage.clear()
    // Reset class attribute (next-themes writes html.classList).
    document.documentElement.className = ''
    document.documentElement.removeAttribute('style')
  })

  it('renders children unchanged', () => {
    render(
      <ThemeProvider attribute="class" defaultTheme="system">
        <div>child-content</div>
      </ThemeProvider>
    )
    expect(screen.getByText('child-content')).toBeInTheDocument()
  })

  it('provides theme context to descendants', () => {
    render(
      <ThemeProvider attribute="class" defaultTheme="light">
        <ThemeConsumer />
      </ThemeProvider>
    )
    // Default theme is propagated through context.
    expect(screen.getByTestId('theme').textContent).toBe('light')
  })

  it('switches theme via setTheme (dark branch)', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider attribute="class" defaultTheme="light">
        <ThemeConsumer />
      </ThemeProvider>
    )

    expect(screen.getByTestId('theme').textContent).toBe('light')
    await user.click(screen.getByText('dark'))

    expect(screen.getByTestId('theme').textContent).toBe('dark')
    expect(screen.getByTestId('resolved-theme').textContent).toBe('dark')
  })

  it('switches theme via setTheme (light branch)', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider attribute="class" defaultTheme="dark">
        <ThemeConsumer />
      </ThemeProvider>
    )

    expect(screen.getByTestId('theme').textContent).toBe('dark')
    await user.click(screen.getByText('light'))

    expect(screen.getByTestId('theme').textContent).toBe('light')
    expect(screen.getByTestId('resolved-theme').textContent).toBe('light')
  })

  it('persists the picked theme to localStorage', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
        <ThemeConsumer />
      </ThemeProvider>
    )

    await user.click(screen.getByText('dark'))

    // next-themes' default storage key is "theme".
    expect(localStorage.getItem('theme')).toBe('dark')
  })

  it('persistence survives unmount + remount (cross-page theme stickiness)', async () => {
    const user = userEvent.setup()
    const { unmount } = render(
      <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
        <ThemeConsumer />
      </ThemeProvider>
    )

    await user.click(screen.getByText('dark'))
    expect(localStorage.getItem('theme')).toBe('dark')
    unmount()

    // Fresh mount must read the persisted value, NOT defaultTheme.
    render(
      <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
        <ThemeConsumer />
      </ThemeProvider>
    )
    expect(screen.getByTestId('theme').textContent).toBe('dark')
  })

  it('applies the active class to document element via attribute="class"', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider attribute="class" defaultTheme="light">
        <ThemeConsumer />
      </ThemeProvider>
    )

    await user.click(screen.getByText('dark'))

    // attribute="class" → next-themes adds the theme name as a className
    // on <html>. Pin this so a regression that breaks css-class theming
    // (e.g. swapping attribute to data-theme silently) gets caught.
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it('forwards "system" theme value through context', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
        <ThemeConsumer />
      </ThemeProvider>
    )

    await user.click(screen.getByText('system'))
    expect(screen.getByTestId('theme').textContent).toBe('system')
    // resolvedTheme reflects the actual computed theme (light/dark), so
    // it should be one of those, not "system".
    const resolved = screen.getByTestId('resolved-theme').textContent
    expect(['light', 'dark']).toContain(resolved)
  })

  it('exposes the themes list (default light/dark/system)', () => {
    render(
      <ThemeProvider attribute="class" defaultTheme="light" enableSystem>
        <ThemeConsumer />
      </ThemeProvider>
    )
    const themesText = screen.getByTestId('themes').textContent ?? ''
    expect(themesText.split(',')).toEqual(expect.arrayContaining(['light', 'dark']))
  })

  it('respects a custom storageKey when provided', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider
        attribute="class"
        defaultTheme="light"
        enableSystem
        storageKey="my-theme"
      >
        <ThemeConsumer />
      </ThemeProvider>
    )

    await user.click(screen.getByText('dark'))
    // The custom key wins.
    expect(localStorage.getItem('my-theme')).toBe('dark')
    expect(localStorage.getItem('theme')).toBeNull()
  })
})
