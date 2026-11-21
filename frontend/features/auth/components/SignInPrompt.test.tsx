import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

vi.mock('next/navigation', () => ({
  usePathname: () => '/artists/band',
}))

import { SignInPrompt } from './SignInPrompt'

describe('SignInPrompt', () => {
  it('wraps the whole prompt in a no-snippet span when asked', () => {
    render(
      <SignInPrompt testId="prompt" noSnippet>
        to join the discussion.
      </SignInPrompt>,
    )
    const span = screen.getByTestId('prompt').querySelector('span[data-nosnippet]')
    expect(span?.textContent).toBe('Sign in to join the discussion.')
    expect(screen.getByRole('link', { name: 'Sign in' }).closest('span[data-nosnippet]')).toBe(span)
  })

  it('carries no attribute by default', () => {
    render(<SignInPrompt testId="prompt">to add a note.</SignInPrompt>)
    expect(screen.getByTestId('prompt').textContent).toBe('Sign in to add a note.')
    expect(screen.getByTestId('prompt').querySelector('[data-nosnippet]')).toBeNull()
  })
})
