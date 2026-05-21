import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardAction,
  CardContent,
  CardFooter,
} from './card'

describe('Card', () => {
  it('renders the full card structure without throwing', () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>Title</CardTitle>
          <CardDescription>Description</CardDescription>
          <CardAction>Action</CardAction>
        </CardHeader>
        <CardContent>Content</CardContent>
        <CardFooter>Footer</CardFooter>
      </Card>
    )
    expect(screen.getByText('Title')).toBeInTheDocument()
    expect(screen.getByText('Description')).toBeInTheDocument()
    expect(screen.getByText('Action')).toBeInTheDocument()
    expect(screen.getByText('Content')).toBeInTheDocument()
    expect(screen.getByText('Footer')).toBeInTheDocument()
  })

  it('tags each sub-component with its data-slot', () => {
    const { container } = render(
      <Card>
        <CardHeader>
          <CardTitle>Title</CardTitle>
          <CardDescription>Description</CardDescription>
          <CardAction>Action</CardAction>
        </CardHeader>
        <CardContent>Content</CardContent>
        <CardFooter>Footer</CardFooter>
      </Card>
    )
    expect(container.querySelector('[data-slot="card"]')).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="card-header"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="card-title"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="card-description"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="card-action"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="card-content"]')
    ).toBeInTheDocument()
    expect(
      container.querySelector('[data-slot="card-footer"]')
    ).toBeInTheDocument()
  })

  it('merges a custom className on the root', () => {
    const { container } = render(<Card className="custom-class">Body</Card>)
    expect(container.querySelector('[data-slot="card"]')).toHaveClass(
      'custom-class'
    )
  })
})
