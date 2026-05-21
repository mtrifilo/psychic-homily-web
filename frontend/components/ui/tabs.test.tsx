import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from './tabs'

function renderTabs(extra?: { className?: string }) {
  return render(
    <Tabs defaultValue="one" className={extra?.className}>
      <TabsList>
        <TabsTrigger value="one">One</TabsTrigger>
        <TabsTrigger value="two" disabled>
          Two
        </TabsTrigger>
      </TabsList>
      <TabsContent value="one">First panel</TabsContent>
      <TabsContent value="two">Second panel</TabsContent>
    </Tabs>
  )
}

describe('Tabs', () => {
  it('renders triggers and the active panel without throwing', () => {
    renderTabs()
    expect(screen.getByRole('tab', { name: 'One' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Two' })).toBeInTheDocument()
    expect(screen.getByText('First panel')).toBeInTheDocument()
  })

  it('marks the default tab as selected', () => {
    renderTabs()
    expect(screen.getByRole('tab', { name: 'One' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })

  it('honors the disabled prop on a trigger', () => {
    renderTabs()
    expect(screen.getByRole('tab', { name: 'Two' })).toBeDisabled()
  })

  it('merges a custom className on the root', () => {
    const { container } = renderTabs({ className: 'custom-class' })
    expect(container.querySelector('[data-slot="tabs"]')).toHaveClass(
      'custom-class'
    )
  })
})
