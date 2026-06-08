import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AppShell } from './AppShell'

vi.mock('./TopBar', () => ({
  TopBar: () => <div data-testid="topbar" />,
}))

vi.mock('./CommandPalette', () => ({
  CommandPalette: () => <div data-testid="command-palette" />,
}))

describe('AppShell', () => {
  it('renders children', () => {
    render(
      <AppShell>
        <div>test content</div>
      </AppShell>
    )
    expect(screen.getByText('test content')).toBeInTheDocument()
  })

  it('renders the TopBar', () => {
    render(
      <AppShell>
        <div>content</div>
      </AppShell>
    )
    expect(screen.getByTestId('topbar')).toBeInTheDocument()
  })

  it('mounts the CommandPalette once so global ⌘K works on every route', () => {
    render(
      <AppShell>
        <div>content</div>
      </AppShell>
    )
    expect(screen.getByTestId('command-palette')).toBeInTheDocument()
  })

  it('renders a skip link to #main-content as the first focusable element', () => {
    render(
      <AppShell>
        <div>content</div>
      </AppShell>
    )
    const skip = screen.getByRole('link', { name: 'Skip to content' })
    expect(skip).toHaveAttribute('href', '#main-content')
    // It must come before the top bar in the document so keyboard users hit it
    // first when tabbing into the page.
    const topbar = screen.getByTestId('topbar')
    expect(
      skip.compareDocumentPosition(topbar) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })
})
