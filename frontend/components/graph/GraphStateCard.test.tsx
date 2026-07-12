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

  it('renders a link-out only when both href and label are provided', () => {
    const { rerender } = render(
      <GraphStateCard message="Best on a larger screen." linkHref="/scenes/phoenix-az" linkLabel="See the scene →" />,
    )
    expect(screen.getByRole('link', { name: 'See the scene →' })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az',
    )
    rerender(<GraphStateCard message="Best on a larger screen." />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
