import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

vi.mock('./HomeDateline', () => ({
  HomeDateline: ({ form, className }: { form: string; className?: string }) => (
    <p data-testid={`dateline-${form}`} className={className} />
  ),
}))
vi.mock('./scrying-grid/ScryingGridWordmark', () => ({
  ScryingGridWordmark: (props: Record<string, unknown>) => (
    <div
      data-testid="wordmark"
      data-preset={String(props.preset)}
      data-motion={String(props.motion)}
      data-heading={String(props.heading)}
      data-lines={JSON.stringify(props.lines)}
    />
  ),
}))

import { HomeNameplate } from './HomeNameplate'

describe('HomeNameplate', () => {
  // jsdom applies no stylesheet: the breakpoint classes are the contract.
  it('is the short dateline alone below xl, where the bar carries the wordmark', () => {
    render(<HomeNameplate />)

    expect(screen.getByTestId('dateline-short')).toHaveClass('xl:hidden')
    // Narrower than the board, the line may wrap; its height is reserved.
    expect(screen.getByTestId('dateline-short')).toHaveClass('max-[389px]:min-h-11')
    expect(screen.getByTestId('wordmark').parentElement).toHaveClass('hidden', 'xl:block')
  })

  it('sets the one-line nameplate from xl: settle motion, no second h1, the long dateline beneath', () => {
    render(<HomeNameplate />)

    const wordmark = screen.getByTestId('wordmark')
    expect(wordmark).toHaveAttribute('data-preset', 'nameplate')
    expect(wordmark).toHaveAttribute('data-motion', 'settle')
    expect(wordmark).toHaveAttribute('data-heading', 'false')
    expect(wordmark).toHaveAttribute('data-lines', '["PSYCHIC HOMILY"]')
    expect(wordmark.parentElement).toContainElement(screen.getByTestId('dateline-long'))
  })
})
