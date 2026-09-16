import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MiniAtlasLoadError } from './VenueMiniAtlasPane'

/**
 * The chunk-load failure branch, which `next/dynamic` renders in place of the
 * map when the MapLibre chunk cannot be fetched. It is unreachable from a
 * rendered pane in jsdom (the dynamic boundary resolves the module), so the
 * callback's output is exercised directly.
 */
describe('MiniAtlasLoadError', () => {
  it('announces the failure and says the rooms are still listed', () => {
    render(<MiniAtlasLoadError />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(/map couldn’t load|map couldn't load/i)
    expect(alert).toHaveTextContent(/every room is in the table/i)
  })

  it('offers a retry only when the boundary gave it one', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    const { rerender } = render(<MiniAtlasLoadError onRetry={onRetry} />)

    await user.click(screen.getByRole('button', { name: /try again/i }))
    expect(onRetry).toHaveBeenCalledTimes(1)

    // A rotated chunk is not always retryable; with no handler the pane says
    // what happened and stops there rather than offering a dead control.
    rerender(<MiniAtlasLoadError />)
    expect(screen.queryByRole('button', { name: /try again/i })).toBeNull()
  })
})
