import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AppShell } from './AppShell'
import { BOTTOM_TAB_BAR_BOX } from '@/test/layoutContracts'

// AppShell is an async Server Component that resolves the effective nav mode
// from the authenticated account first, then the nav-mode cookie (PSY-1117).
let cookieValue: string | undefined
let accountNavMode: string | undefined
vi.mock('next/headers', () => ({
  cookies: () =>
    Promise.resolve({
      get: (name: string) =>
        name === 'nav_mode' && cookieValue ? { value: cookieValue } : undefined,
    }),
}))
vi.mock('@/lib/auth-hydration', () => ({
  getAuthenticatedNavMode: () => Promise.resolve(accountNavMode),
}))

vi.mock('./TopBar', () => ({
  TopBar: ({ variant = 'full' }: { variant?: string }) => (
    <div data-testid="topbar" data-variant={variant} />
  ),
}))
vi.mock('./CommandPalette', () => ({
  CommandPalette: () => <div data-testid="command-palette" />,
}))
vi.mock('./SideNavShell', () => ({
  SideNavShell: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="side-nav-shell">{children}</div>
  ),
}))
vi.mock('./nav/BottomTabBar', () => ({
  BottomTabBar: () => <div data-testid="bottom-tab-bar" />,
}))

// Async Server Component: resolve the element, then render it.
async function renderShell(children: React.ReactNode = <div>test content</div>) {
  return render(await AppShell({ children }))
}

describe('AppShell', () => {
  beforeEach(() => {
    cookieValue = undefined
    accountNavMode = undefined
  })

  it('renders children', async () => {
    await renderShell(<div>test content</div>)
    expect(screen.getByText('test content')).toBeInTheDocument()
  })

  it('defaults to top-nav mode: full TopBar, no side-nav shell', async () => {
    await renderShell()
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'full')
    expect(screen.queryByTestId('side-nav-shell')).not.toBeInTheDocument()
  })

  it('renders side-nav mode when nav_mode=side: slim TopBar + SideNavShell wrapping content', async () => {
    cookieValue = 'side'
    await renderShell(<div>page</div>)
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'slim')
    const shell = screen.getByTestId('side-nav-shell')
    expect(shell).toBeInTheDocument()
    expect(shell).toHaveTextContent('page')
  })

  it('treats an unknown nav_mode cookie value as top mode', async () => {
    cookieValue = 'bogus'
    await renderShell()
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'full')
    expect(screen.queryByTestId('side-nav-shell')).not.toBeInTheDocument()
  })

  it('renders the account preference when authenticated, with no cookie set', async () => {
    accountNavMode = 'side'
    await renderShell(<div>page</div>)
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'slim')
    expect(screen.getByTestId('side-nav-shell')).toBeInTheDocument()
  })

  it('account preference wins over a conflicting cookie (account side, cookie top)', async () => {
    accountNavMode = 'side'
    cookieValue = 'top'
    await renderShell(<div>page</div>)
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'slim')
    expect(screen.getByTestId('side-nav-shell')).toBeInTheDocument()
  })

  it('account preference wins over a conflicting cookie (account top, cookie side)', async () => {
    accountNavMode = 'top'
    cookieValue = 'side'
    await renderShell()
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'full')
    expect(screen.queryByTestId('side-nav-shell')).not.toBeInTheDocument()
  })

  it('falls back to the cookie when there is no account preference (anonymous)', async () => {
    accountNavMode = undefined
    cookieValue = 'side'
    await renderShell(<div>page</div>)
    expect(screen.getByTestId('topbar')).toHaveAttribute('data-variant', 'slim')
    expect(screen.getByTestId('side-nav-shell')).toBeInTheDocument()
  })

  // PSY-1020: the bar is the primary mobile nav, mounted once in the shell so
  // it persists on every route (it hides itself at `xl`, where PrimaryNav appears).
  it('mounts the BottomTabBar and pads the shell so content clears the fixed bar', async () => {
    const { container } = await renderShell(<div>content</div>)
    expect(screen.getByTestId('bottom-tab-bar')).toBeInTheDocument()
    // The reservation itself, not just its xl release — deleting the pb-[calc]
    // leaves every mobile footer under the fixed bar with no failing test.
    // BOTTOM_TAB_BAR_BOX is the shared copy BottomTabBar.test.tsx asserts as
    // the bar's rendered `h-[…]`; reserving and rendering must not drift.
    expect(container.firstChild).toHaveClass(
      `pb-[${BOTTOM_TAB_BAR_BOX}]`,
      'xl:pb-0'
    )
  })

  // PSY-1820: viewport-fit=cover makes the landscape notch insets nonzero, and
  // the shell is where in-flow content absorbs them. It lives here rather than
  // on `body` because Radix's react-remove-scroll overwrites body padding
  // whenever an overlay opens — see the comment in AppShell.tsx.
  it('insets in-flow content from the landscape safe area', async () => {
    const { container } = await renderShell(<div>content</div>)
    expect(container.firstChild).toHaveClass(
      'pl-[env(safe-area-inset-left)]',
      'pr-[env(safe-area-inset-right)]'
    )
  })

  it('mounts the BottomTabBar in side-nav mode too — the bar is xl:hidden, so the desktop nav preference must not remove mobile nav', async () => {
    cookieValue = 'side'
    await renderShell(<div>content</div>)
    expect(screen.getByTestId('bottom-tab-bar')).toBeInTheDocument()
  })

  it('mounts the CommandPalette once so global ⌘K works on every route', async () => {
    await renderShell()
    expect(screen.getByTestId('command-palette')).toBeInTheDocument()
  })

  it('renders a skip link to #main-content as the first focusable element', async () => {
    await renderShell()
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
