import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { StatsList } from './StatsList'

describe('StatsList', () => {
  const items = [
    { label: 'Releases', value: 4 },
    { label: 'Labels', value: 2 },
    { label: 'Shows tracked', value: 13 },
  ]

  it('renders nothing when items is empty', () => {
    const { container } = render(<StatsList items={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders each label and value in the sidebar variant', () => {
    render(<StatsList items={items} />)
    expect(screen.getByText('Releases')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(screen.getByText('Labels')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('Shows tracked')).toBeInTheDocument()
    expect(screen.getByText('13')).toBeInTheDocument()
  })

  it('renders as a <dl> in sidebar variant', () => {
    const { container } = render(<StatsList items={items} />)
    expect(container.querySelector('dl')).toBeInTheDocument()
    expect(container.querySelectorAll('dt')).toHaveLength(3)
    expect(container.querySelectorAll('dd')).toHaveLength(3)
  })

  it('formats numeric values with thousands separators', () => {
    render(<StatsList items={[{ label: 'Releases', value: 1234 }]} />)
    expect(screen.getByText('1,234')).toBeInTheDocument()
  })

  it('renders string values as-is (no number formatting)', () => {
    render(<StatsList items={[{ label: 'Status', value: 'Active' }]} />)
    expect(screen.getByText('Active')).toBeInTheDocument()
  })

  it('renders ReactNode values as-is (e.g. links)', () => {
    render(
      <StatsList
        items={[
          {
            label: 'Last show',
            // Absolute href keeps this a plain anchor (the test only proves
            // ReactNode pass-through); an internal "/shows/..." path would
            // trip @next/next/no-html-link-for-pages, which is irrelevant here.
            value: <a href="https://example.com/shows/x">May 17 at Valley Bar</a>,
          },
        ]}
      />
    )
    expect(
      screen.getByRole('link', { name: 'May 17 at Valley Bar' })
    ).toBeInTheDocument()
  })

  it('renders inline variant as a single paragraph with middots', () => {
    const { container } = render(<StatsList items={items} variant="inline" />)
    expect(container.querySelector('p')).toBeInTheDocument()
    expect(container.querySelector('dl')).not.toBeInTheDocument()
    // 3 items => 2 middot separators
    const middots = container.querySelectorAll('span[aria-hidden="true"]')
    expect(middots).toHaveLength(2)
  })

  it('inline variant lowercases labels for natural prose reading', () => {
    render(<StatsList items={items} variant="inline" />)
    expect(screen.getByText('releases')).toBeInTheDocument()
    expect(screen.getByText('shows tracked')).toBeInTheDocument()
  })

  it('forwards custom className', () => {
    const { container } = render(
      <StatsList items={items} className="mt-4" />
    )
    expect((container.firstChild as HTMLElement).className).toContain('mt-4')
  })
})
