import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Alert, AlertTitle, AlertDescription } from './alert'

describe('Alert', () => {
  it('renders its content with the alert role', () => {
    render(
      <Alert>
        <AlertTitle>Heads up</AlertTitle>
        <AlertDescription>Something happened</AlertDescription>
      </Alert>
    )
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Heads up')).toBeInTheDocument()
    expect(screen.getByText('Something happened')).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Alert className="custom-class">Body</Alert>)
    expect(screen.getByRole('alert')).toHaveClass('custom-class')
  })

  it('applies the default variant classes', () => {
    render(<Alert>Body</Alert>)
    expect(screen.getByRole('alert')).toHaveClass('bg-card')
  })

  it('applies the destructive variant classes', () => {
    render(<Alert variant="destructive">Body</Alert>)
    expect(screen.getByRole('alert')).toHaveClass('text-destructive')
  })

  it('tags sub-components with their data-slot', () => {
    const { container } = render(
      <Alert>
        <AlertTitle>Title</AlertTitle>
        <AlertDescription>Description</AlertDescription>
      </Alert>
    )
    expect(
      container.querySelector('[data-slot="alert-title"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="alert-description"]')
    ).toBeInTheDocument()
  })
})
