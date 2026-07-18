import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ArchiveMasthead } from './ArchiveMasthead'

describe('ArchiveMasthead', () => {
  const now = new Date('2026-07-18T12:00:00Z')

  it('renders the year archive title, ARCHIVE marker, and quarter nav', () => {
    render(<ArchiveMasthead window="2026" now={now} />)

    expect(
      screen.getByRole('heading', { name: 'Charts — 2026' })
    ).toBeInTheDocument()
    expect(screen.getByText('ARCHIVE')).toBeInTheDocument()
    expect(screen.getByLabelText('Adjacent chart periods')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Q1' })).toHaveAttribute(
      'href',
      '/charts/2026/q1'
    )
    expect(screen.getByText(/viewing the full year/)).toBeInTheDocument()
  })

  it('highlights the current quarter and links back to the year', () => {
    render(<ArchiveMasthead window="2026-q2" now={now} />)

    expect(
      screen.getByRole('heading', { name: 'Charts — Q2 2026' })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Q2' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(screen.getByRole('link', { name: 'full year' })).toHaveAttribute(
      'href',
      '/charts/2026'
    )
  })
})
