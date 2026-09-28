import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LoginPromptDialog } from './LoginPromptDialog'

describe('LoginPromptDialog', () => {
  // Each button goes where its label says: Create account to the href that
  // opens that tab, Sign in to the one that does not.
  it('puts each destination on the button that names it', () => {
    render(
      <LoginPromptDialog
        open
        onOpenChange={vi.fn()}
        hrefs={{
          signInHref: '/auth?returnTo=%2Fshows%2Fexample',
          createAccountHref: '/auth?returnTo=%2Fshows%2Fexample&intent=report',
        }}
      />
    )

    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/auth?returnTo=%2Fshows%2Fexample'
    )
    expect(screen.getByRole('link', { name: 'Create account' })).toHaveAttribute(
      'href',
      '/auth?returnTo=%2Fshows%2Fexample&intent=report'
    )
  })
})
