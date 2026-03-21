import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ProfileSections } from './ProfileSections'
import type { ProfileSectionResponse } from '@/features/auth'

function makeSection(
  overrides: Partial<ProfileSectionResponse> = {}
): ProfileSectionResponse {
  return {
    id: 1,
    title: 'About Me',
    content: 'I love live music.',
    position: 0,
    is_visible: true,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('ProfileSections', () => {
  it('returns null when all sections are hidden', () => {
    const { container } = render(
      <ProfileSections
        sections={[makeSection({ is_visible: false })]}
      />
    )
    expect(container.firstChild).toBeNull()
  })

  it('returns null for empty sections array', () => {
    const { container } = render(<ProfileSections sections={[]} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders visible sections', () => {
    render(
      <ProfileSections
        sections={[
          makeSection({ id: 1, title: 'About Me', content: 'Music fan.' }),
          makeSection({
            id: 2,
            title: 'Favorite Genres',
            content: 'Punk, shoegaze.',
            position: 1,
          }),
        ]}
      />
    )
    expect(screen.getByText('About Me')).toBeInTheDocument()
    expect(screen.getByText('Music fan.')).toBeInTheDocument()
    expect(screen.getByText('Favorite Genres')).toBeInTheDocument()
    expect(screen.getByText('Punk, shoegaze.')).toBeInTheDocument()
  })

  it('filters out invisible sections', () => {
    render(
      <ProfileSections
        sections={[
          makeSection({ id: 1, title: 'Visible Section', is_visible: true }),
          makeSection({ id: 2, title: 'Hidden Section', is_visible: false }),
        ]}
      />
    )
    expect(screen.getByText('Visible Section')).toBeInTheDocument()
    expect(screen.queryByText('Hidden Section')).not.toBeInTheDocument()
  })

  it('sorts sections by position', () => {
    render(
      <ProfileSections
        sections={[
          makeSection({ id: 1, title: 'Second', position: 1 }),
          makeSection({ id: 2, title: 'First', position: 0 }),
          makeSection({ id: 3, title: 'Third', position: 2 }),
        ]}
      />
    )
    // CardTitle renders as a div with data-slot="card-title"
    const titles = document.querySelectorAll('[data-slot="card-title"]')
    expect(titles[0]).toHaveTextContent('First')
    expect(titles[1]).toHaveTextContent('Second')
    expect(titles[2]).toHaveTextContent('Third')
  })

  it('renders content in whitespace-pre-wrap element', () => {
    render(
      <ProfileSections
        sections={[
          makeSection({
            content: 'Line 1\nLine 2\nLine 3',
          }),
        ]}
      />
    )
    // The content p element has whitespace-pre-wrap class
    const contentEl = document.querySelector('.whitespace-pre-wrap')
    expect(contentEl).toBeInTheDocument()
    expect(contentEl?.textContent).toBe('Line 1\nLine 2\nLine 3')
  })
})
