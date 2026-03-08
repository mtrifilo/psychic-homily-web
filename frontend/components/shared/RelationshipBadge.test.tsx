import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RelationshipBadge, type RelationshipType } from './RelationshipBadge'

describe('RelationshipBadge', () => {
  it('renders default label for label-mate type', () => {
    render(<RelationshipBadge type="label-mate" />)
    expect(screen.getByText('label mate')).toBeInTheDocument()
  })

  it('renders default label for similar type', () => {
    render(<RelationshipBadge type="similar" />)
    expect(screen.getByText('similar')).toBeInTheDocument()
  })

  it('renders default label for shared-bills type', () => {
    render(<RelationshipBadge type="shared-bills" />)
    expect(screen.getByText('shared bills')).toBeInTheDocument()
  })

  it('renders default label for side-project type', () => {
    render(<RelationshipBadge type="side-project" />)
    expect(screen.getByText('side project')).toBeInTheDocument()
  })

  it('renders default label for member-of type', () => {
    render(<RelationshipBadge type="member-of" />)
    expect(screen.getByText('member of')).toBeInTheDocument()
  })

  it('renders default label for formerly type', () => {
    render(<RelationshipBadge type="formerly" />)
    expect(screen.getByText('formerly')).toBeInTheDocument()
  })

  it('uses custom label when provided', () => {
    render(<RelationshipBadge type="side-project" label="side project of" />)
    expect(screen.getByText('side project of')).toBeInTheDocument()
    expect(screen.queryByText('side project')).not.toBeInTheDocument()
  })

  it('formats shared-bills with singular count', () => {
    render(<RelationshipBadge type="shared-bills" count={1} />)
    expect(screen.getByText('shared 1 bill')).toBeInTheDocument()
  })

  it('formats shared-bills with plural count', () => {
    render(<RelationshipBadge type="shared-bills" count={3} />)
    expect(screen.getByText('shared 3 bills')).toBeInTheDocument()
  })

  it('formats shared-bills with zero count', () => {
    render(<RelationshipBadge type="shared-bills" count={0} />)
    expect(screen.getByText('shared 0 bills')).toBeInTheDocument()
  })

  it('ignores count for non-shared-bills types', () => {
    render(<RelationshipBadge type="similar" count={5} />)
    expect(screen.getByText('similar')).toBeInTheDocument()
    expect(screen.queryByText(/5/)).not.toBeInTheDocument()
  })

  it('renders as a span element', () => {
    render(<RelationshipBadge type="label-mate" />)
    const badge = screen.getByText('label mate')
    expect(badge.tagName).toBe('SPAN')
  })

  it('applies custom className', () => {
    render(<RelationshipBadge type="similar" className="ml-2" />)
    const badge = screen.getByText('similar')
    expect(badge.className).toContain('ml-2')
  })

  it('count formatting takes precedence over custom label for shared-bills', () => {
    // When type is shared-bills and count is provided, the formatted count label is used
    // regardless of the custom label prop
    render(<RelationshipBadge type="shared-bills" count={3} label="on the same bills" />)
    expect(screen.getByText('shared 3 bills')).toBeInTheDocument()
  })

  it('custom label is used for shared-bills when no count is provided', () => {
    render(<RelationshipBadge type="shared-bills" label="on the same bills" />)
    expect(screen.getByText('on the same bills')).toBeInTheDocument()
  })
})
