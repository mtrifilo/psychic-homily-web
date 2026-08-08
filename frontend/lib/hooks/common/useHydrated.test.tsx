import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { useHydrated } from './useHydrated'

function HydrationProbe() {
  return <span>{useHydrated() ? 'browser' : 'server'}</span>
}

describe('useHydrated', () => {
  // The half a client-only test cannot reach, and the half that matters: the
  // server render must produce the UNREFINED answer, or every caller that
  // gates markup on this hook emits HTML the hydration render disagrees with.
  it('is false in a server render', () => {
    expect(renderToString(<HydrationProbe />)).toContain('server')
  })

  it('is true once the browser is running it', () => {
    render(<HydrationProbe />)
    expect(screen.getByText('browser')).toBeInTheDocument()
  })
})
