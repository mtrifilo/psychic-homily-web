import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ENTITY_LINK_CLASS } from '@/components/shared/entityLink'
import { ShowBill } from './ShowBill'

const BILL = [
  { id: 1, slug: 'lead', name: 'Lead Act' },
  { id: 2, slug: 'opener', name: 'Opening Act' },
]

describe('ShowBill link treatment', () => {
  it('keeps its own hover styling when no treatment is asked for', () => {
    render(<ShowBill artists={BILL} isCancelled={false} isSoldOut={false} />)
    expect(screen.getByRole('link', { name: 'Lead Act' }).className).toBe(
      'hover:text-primary hover:underline'
    )
    expect(screen.getByRole('link', { name: 'Opening Act' }).className).toBe(
      'hover:text-foreground hover:underline'
    )
  })

  it('applies the restrained entity-link treatment to every name on opt-in', () => {
    render(
      <ShowBill
        artists={BILL}
        isCancelled={false}
        isSoldOut={false}
        linkTreatment="restrained"
      />
    )
    for (const name of ['Lead Act', 'Opening Act']) {
      expect(screen.getByRole('link', { name }).className).toBe(
        ENTITY_LINK_CLASS.restrained
      )
    }
  })
})

// A caller reorders `afterBill` with flex `order` and `basis` (the artist
// table's stacked phone row), which holds only while it is a direct item of
// the bill's wrapping flex row.
describe('ShowBill afterBill slot', () => {
  it('is a direct item of the wrapping flex row, before the badges', () => {
    render(
      <ShowBill
        artists={BILL}
        isCancelled
        isSoldOut={false}
        afterBill={<span data-testid="after">· The Room</span>}
      />
    )
    const after = screen.getByTestId('after')
    const row = after.parentElement!
    expect(row).toHaveClass('flex', 'flex-wrap')
    const items = [...row.children]
    expect(items.indexOf(after)).toBeLessThan(
      items.indexOf(screen.getByText('CANCELLED'))
    )
  })
})
