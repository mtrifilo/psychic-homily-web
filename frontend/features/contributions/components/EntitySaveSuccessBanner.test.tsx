import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EntitySaveSuccessBanner } from './EntitySaveSuccessBanner'

describe('EntitySaveSuccessBanner', () => {
  it('renders nothing when not visible', () => {
    const { container } = render(<EntitySaveSuccessBanner visible={false} />)

    expect(container).toBeEmptyDOMElement()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('renders the default "Changes saved" copy when visible and no message is supplied', () => {
    render(<EntitySaveSuccessBanner visible={true} />)

    const banner = screen.getByRole('status')
    expect(banner).toBeInTheDocument()
    expect(banner).toHaveTextContent('Changes saved')
  })

  // PSY-622: optional `message` prop generalizes the banner so admin
  // moderation surfaces (Approve / Reject in ModerationQueue, future
  // Resolve-report / Hide-field-note flows) can reuse the same primitive
  // instead of forking a near-identical banner per call site.
  it('renders the supplied admin-flow message when one is provided', () => {
    render(
      <EntitySaveSuccessBanner
        visible={true}
        message="Approved — change applied to Test Artist"
      />
    )

    const banner = screen.getByRole('status')
    expect(banner).toHaveTextContent(/Approved/)
    expect(banner).toHaveTextContent('Test Artist')
    // Default copy should be replaced, not appended.
    expect(banner).not.toHaveTextContent('Changes saved')
  })

  it('uses aria-live="polite" so screen readers announce the success without interrupting', () => {
    render(<EntitySaveSuccessBanner visible={true} />)

    expect(screen.getByRole('status')).toHaveAttribute('aria-live', 'polite')
  })
})
