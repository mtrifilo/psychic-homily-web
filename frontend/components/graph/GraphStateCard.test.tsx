import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GraphStateCard } from './GraphStateCard'

describe('GraphStateCard', () => {
  it('renders the message and forwards className sizing', () => {
    const { container } = render(
      <GraphStateCard message="This view couldn't load." className="h-[240px]" />,
    )
    expect(screen.getByText("This view couldn't load.")).toBeInTheDocument()
    const box = container.firstElementChild!
    expect(box.className).toContain('h-[240px]')
    expect(box.className).toContain('border')
  })

  it('announces error states via role="alert"', () => {
    render(<GraphStateCard role="alert" message="This view couldn't load." />)
    expect(screen.getByRole('alert')).toHaveTextContent("couldn't load")
  })

  // The card is a terminal state, not a way forward: the sub-640px form of a
  // graph section is MobileGraphTeaser, and that is where the link lives.
  it('offers no link of its own', () => {
    render(<GraphStateCard message="This view couldn't load." />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
