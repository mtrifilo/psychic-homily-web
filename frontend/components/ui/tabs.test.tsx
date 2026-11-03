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

    it('styles the unselected label only through base tokens, so it stays muted Medium in both modes', () => {
      renderTwoTabs()
      const unselected = trigger('Create account')
      expect(unselected).toHaveAttribute('data-state', 'inactive')
      expect(unselected).toHaveClass(...BOTH_STATES_TOKENS, ...UNSELECTED_TOKENS)
      // Every label colour or weight token other than the base ones must be
      // scoped to the selected state; a dark: or inactive-state label token
      // would change the unselected label in one mode.
      const labelTokens = Array.from(unselected.classList).filter(token =>
        /^(text-(?!sm$)|font-)/.test(token.split(':').pop() ?? '')
      )
      const unscoped = labelTokens.filter(
        token => !token.startsWith('data-[state=active]:')
      )
      expect(unscoped).toEqual(['text-muted-foreground', 'font-medium'])
    })

    it('keeps the focus border above the selected boundary colour', () => {
      renderTwoTabs()
      expect(trigger('Sign in')).toHaveClass(
        'focus-visible:data-[state=active]:border-ring',
        'focus-visible:ring-[3px]',
        'focus-visible:outline-1'
      )
    })

    it('lets an underline-list consumer pass border-0 border-b-2 to drop the boundary and keep the underline', () => {
      renderTwoTabs('border-0 border-b-2')
      const selected = trigger('Sign in')
      expect(selected).toHaveClass(
        'border-0',
        'border-b-2',
        'data-[state=active]:border-b-primary'
      )
      expect(selected).not.toHaveClass('border')
    })

    it.each(['Sign in', 'Create account'])(
      'reserves the SemiBold width of %s with a hidden copy in the same grid cell',
      name => {
        renderTwoTabs()
        const tab = trigger(name)
        const cell = tab.querySelector('[data-slot="tabs-trigger-label"]')
        expect(cell).toHaveClass(
          'inline-grid',
          '*:col-start-1',
          '*:row-start-1'
        )
        expect(cell?.children).toHaveLength(2)
        const [reserve, label] = Array.from(cell?.children ?? [])
        expect(reserve).toHaveAttribute('aria-hidden', 'true')
        expect(reserve).toHaveClass(
          'invisible',
          'font-semibold',
          'select-none',
          'pointer-events-none'
        )
        expect(reserve).toHaveTextContent(name)
        expect(label).not.toHaveAttribute('aria-hidden')
        expect(label).toHaveTextContent(name)
        expect(tab).toHaveAccessibleName(name)
      }
    )

    it('passes an asChild trigger its single child without the reservation', () => {
      render(
        <Tabs defaultValue="one">
          <TabsList>
            <TabsTrigger value="one" asChild>
              <a href="#one">One</a>
            </TabsTrigger>
          </TabsList>
        </Tabs>
      )
      const tab = screen.getByRole('tab', { name: 'One' })
      expect(tab.tagName).toBe('A')
      expect(
        tab.querySelector('[data-slot="tabs-trigger-label"]')
      ).not.toBeInTheDocument()
    })
  })
})
