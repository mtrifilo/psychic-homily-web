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

  describe('selected-state indicators', () => {
    // Class tokens are the contract here: jsdom computes no Tailwind styles,
    // and a regenerated shadcn trigger would drop these exact tokens.
    const BOTH_STATES_TOKENS = ['border', 'border-b-2']
    const SELECTED_TOKENS = [
      'data-[state=active]:border-muted-foreground',
      'data-[state=active]:border-b-primary',
      'data-[state=active]:text-foreground',
      'data-[state=active]:font-semibold',
      'data-[state=active]:bg-background',
      'dark:data-[state=active]:bg-input/30',
    ]
    const UNSELECTED_TOKENS = [
      'text-muted-foreground',
      'font-medium',
      'border-transparent',
    ]

    // Two enabled triggers: renderTabs disables its second one.
    function renderTwoTabs(triggerClassName?: string) {
      return render(
        <Tabs defaultValue="sign-in">
          <TabsList>
            <TabsTrigger value="sign-in" className={triggerClassName}>
              Sign in
            </TabsTrigger>
            <TabsTrigger value="create" className={triggerClassName}>
              Create account
            </TabsTrigger>
          </TabsList>
          <TabsContent value="sign-in">Sign in panel</TabsContent>
          <TabsContent value="create">Create panel</TabsContent>
        </Tabs>
      )
    }

    function trigger(name: string) {
      return screen.getByRole('tab', { name })
    }

    it('gives the selected trigger the boundary, the underline, the foreground label and the heavier weight', () => {
      renderTwoTabs()
      const selected = trigger('Sign in')
      expect(selected).toHaveAttribute('data-state', 'active')
      expect(selected).toHaveClass(...BOTH_STATES_TOKENS, ...SELECTED_TOKENS)
    })

    it('renders the unselected trigger with a muted medium label and a transparent border in both modes', () => {
      renderTwoTabs()
      const unselected = trigger('Create account')
      expect(unselected).toHaveAttribute('data-state', 'inactive')
      expect(unselected).toHaveClass(...BOTH_STATES_TOKENS, ...UNSELECTED_TOKENS)
      // A bare text-foreground (or a dark-only muted label) would make the
      // unselected label match the selected one in light mode.
      expect(unselected).not.toHaveClass('text-foreground')
      expect(unselected).not.toHaveClass('dark:text-muted-foreground')
      expect(unselected).not.toHaveClass('dark:data-[state=active]:border-input')
    })

    it('lets an underline-list consumer remove the boundary with border-0', () => {
      renderTwoTabs('border-0 border-b-2')
      const selected = trigger('Sign in')
      expect(selected).toHaveClass('border-0', 'border-b-2')
      expect(selected).not.toHaveClass('border')
    })
  })
})
