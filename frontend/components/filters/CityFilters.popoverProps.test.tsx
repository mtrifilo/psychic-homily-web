import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SOFT_KEYBOARD_VIEWPORT_QUERY } from '@/lib/softKeyboardViewport'

/**
 * `avoidCollisions={false}` is the prop that stops the popover flipping over
 * its trigger, and jsdom cannot show it working: with every rect at 0x0 there
 * is no overflow to avoid, so `data-side` reads `bottom` either way. This file
 * mocks the popover primitive and asserts the props the component hands it,
 * which is the part that is deterministic here. Whether those props produce
 * the right geometry is e2e/pages/city-filter-mobile.spec.ts.
 */
type ContentProps = {
  side?: string
  align?: string
  avoidCollisions?: boolean
  className?: string
  children?: React.ReactNode
}

let contentProps: ContentProps | null = null

vi.mock('@/components/ui/popover', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/components/ui/popover')>()
  return {
    ...actual,
    PopoverContent: ({ children, ...props }: ContentProps) => {
      contentProps = props
      return <div data-testid="popover-content-stub">{children}</div>
    },
  }
})

const { CityFilters, SOFT_KEYBOARD_CONTENT_CLASS } = await import('./CityFilters')

const cities = [
  { city: 'Phoenix', state: 'AZ', count: 8 },
  { city: 'Mesa', state: 'AZ', count: 3 },
]

const originalMatchMedia = window.matchMedia

function mockSoftKeyboardViewport(matches: boolean) {
  window.matchMedia = vi.fn(
    (query: string) =>
      ({
        matches: matches && query === SOFT_KEYBOARD_VIEWPORT_QUERY,
        media: query,
      }) as MediaQueryList
  )
}

beforeEach(() => {
  contentProps = null
})

afterEach(() => {
  window.matchMedia = originalMatchMedia
})

async function openFilter() {
  const user = userEvent.setup()
  render(
    <CityFilters cities={cities} selectedCities={[]} onFilterChange={vi.fn()} />
  )
  await user.click(screen.getByTestId('city-filter-combobox'))
}

describe('CityFilters popover props', () => {
  it('turns collision avoidance off on a soft-keyboard viewport', async () => {
    mockSoftKeyboardViewport(true)

    await openFilter()

    expect(contentProps).toMatchObject({
      side: 'bottom',
      align: 'start',
      avoidCollisions: false,
    })
    expect(contentProps?.className).toContain(SOFT_KEYBOARD_CONTENT_CLASS)
  })

  it('leaves collision avoidance on for a pointer viewport', async () => {
    mockSoftKeyboardViewport(false)

    await openFilter()

    expect(contentProps).toMatchObject({
      side: 'bottom',
      align: 'start',
      avoidCollisions: true,
    })
    expect(contentProps?.className).not.toContain(SOFT_KEYBOARD_CONTENT_CLASS)
  })
})
