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
      className={String(props.className)}
    />
  ),
}))

import { HomeNameplate } from './HomeNameplate'

describe('HomeNameplate', () => {
  // jsdom applies no stylesheet: the breakpoint classes are the contract.
  it('is the short dateline alone below sm, where the bar carries the wordmark', () => {
    render(<HomeNameplate />)

    expect(screen.getByTestId('dateline-short')).toHaveClass('sm:hidden')
    // Narrower than the board, the line may wrap; its height is reserved.
    expect(screen.getByTestId('dateline-short')).toHaveClass('max-[429px]:min-h-11')
    expect(screen.getByTestId('wordmark').parentElement).toHaveClass('hidden', 'sm:block')
  })

  it('sets the one-line nameplate from sm, scaled to the column: settle motion, no second h1, the long dateline beneath', () => {
    render(<HomeNameplate />)

    const wordmark = screen.getByTestId('wordmark')
    expect(wordmark).toHaveAttribute('data-preset', 'nameplate')
    expect(wordmark).toHaveAttribute('data-motion', 'settle')
    expect(wordmark).toHaveAttribute('data-heading', 'false')
    expect(wordmark).toHaveAttribute('data-lines', '["PSYCHIC HOMILY"]')
    // 56px at a 640 viewport, 100px from 1429 (the board's 1440 included).
    expect(wordmark).toHaveClass('h-[clamp(56px,7vw,100px)]', 'max-w-full')
    expect(wordmark.parentElement).toContainElement(screen.getByTestId('dateline-long'))
    // The long line may wrap just above sm; its second row is reserved.
    expect(screen.getByTestId('dateline-long')).toHaveClass('max-[669px]:min-h-9')
  })
})
