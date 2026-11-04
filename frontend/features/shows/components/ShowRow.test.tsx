import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { renderToString } from 'react-dom/server'
import { ShowRow, ShowRowSaveButton } from './ShowRow'
import type { ShowResponse } from '../types'

const saveButtonProps = vi.fn()
vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: (props: Record<string, unknown>) => {
    saveButtonProps(props)
    return <button type="button">heart</button>
  },
}))

// Saturday Nov 7 2026, noon in Phoenix.
const NOW = new Date('2026-11-07T19:00:00Z')

function show(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    title: 'Playboy Manbaby at Walter Studios',
    slug: 'playboy-manbaby-walter',
    // 8 PM Nov 7 in Phoenix.
    event_date: '2026-11-08T03:00:00Z',
    state: 'AZ',
    status: 'approved',
    artists: [
      { id: 1, name: 'Playboy Manbaby' },
      { id: 2, name: 'Dan Deacon' },
    ],
    venues: [
      {
        id: 9,
        name: 'Walter Studios',
        slug: 'walter-studios',
        city: 'Phoenix',
        state: 'AZ',
        timezone: 'America/Phoenix',
        verified: true,
      },
    ],
    ...overrides,
  } as ShowResponse
}

function whenColumn() {
  const row = screen.getByRole('article')
  return row.firstElementChild as HTMLElement
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(NOW)
})

afterEach(() => {
  vi.useRealTimers()
  saveButtonProps.mockClear()
})

describe('ShowRow', () => {
  it('says Tonight over the venue-local start time', () => {
    render(<ShowRow show={show()} trailing={null} />)

    const when = whenColumn()
    expect(within(when).getByText('Tonight')).toBeInTheDocument()
    expect(within(when).getByText(/^8:00\sPM$/)).toBeInTheDocument()
  })

  // "Now" is the reader's clock. The server render must not guess it, or its
  // HTML and the hydration render could name different days.
  it('prints the date, not a relative day, in server HTML', () => {
    const html = renderToString(<ShowRow show={show()} trailing={null} />)

    expect(html).toContain('SAT NOV 7')
    expect(html).not.toContain('Tonight')
  })

  it('says Tomorrow for the next venue-local day', () => {
    render(
      <ShowRow
        show={show({ event_date: '2026-11-09T02:30:00Z' })}
        trailing={null}
      />
    )

    expect(within(whenColumn()).getByText('Tomorrow')).toBeInTheDocument()
    expect(within(whenColumn()).getByText(/^7:30\sPM$/)).toBeInTheDocument()
  })

  it('prints the weekday and date two or more days out', () => {
    render(
      <ShowRow
        show={show({ event_date: '2026-11-15T03:00:00Z' })}
        trailing={null}
      />
    )

    expect(within(whenColumn()).getByText('SAT NOV 14')).toBeInTheDocument()
  })

  it('shows the start time at every width', () => {
    render(<ShowRow show={show()} trailing={null} />)

    const time = within(whenColumn()).getByText(/^8:00\sPM$/)
    expect(time.className).not.toMatch(/\bhidden\b/)
    expect(time.className).toContain('tabular-nums')
  })

  it('keeps the date and drops the clock on a guessed zone', () => {
    render(
      <ShowRow
        show={show({
          state: '',
          venues: [
            {
              id: 9,
              name: 'Hall Ohne Zone',
              slug: 'hall',
              city: 'Berlin',
              state: '',
              timezone: null,
              verified: true,
            },
          ],
        })}
        trailing={null}
      />
    )

    const when = whenColumn()
    expect(within(when).queryByText('Tonight')).not.toBeInTheDocument()
    expect(within(when).getByText(/NOV 7/)).toBeInTheDocument()
    expect(within(when).queryByText(/PM/)).not.toBeInTheDocument()
  })

  it('dates a past row and mutes it, even on its own day', () => {
    render(<ShowRow show={show()} past trailing={null} />)

    const date = within(whenColumn()).getByText('SAT NOV 7')
    expect(date.className).toContain('text-muted-foreground')
    expect(
      screen.getByRole('link', { name: 'Playboy Manbaby · Dan Deacon' }).className
    ).toContain('text-muted-foreground')
  })

  it('reads lineup over venue and city, linking both', () => {
    render(<ShowRow show={show()} trailing={null} />)

    expect(
      screen.getByRole('link', { name: 'Playboy Manbaby · Dan Deacon' })
    ).toHaveAttribute('href', '/shows/playboy-manbaby-walter')
    expect(screen.getByRole('link', { name: 'Walter Studios' })).toHaveAttribute(
      'href',
      '/venues/walter-studios'
    )
    expect(screen.getByText(/· Phoenix, AZ/)).toBeInTheDocument()
    expect(screen.getByRole('article')).toHaveAccessibleName(
      'Playboy Manbaby at Walter Studios'
    )
  })

  it('falls back to the show title when no artist is billed', () => {
    render(<ShowRow show={show({ artists: [] })} trailing={null} />)

    expect(
      screen.getByRole('link', { name: 'Playboy Manbaby at Walter Studios' })
    ).toHaveAttribute('href', '/shows/playboy-manbaby-walter')
  })

  it('names a slug-less venue without linking it', () => {
    render(
      <ShowRow
        show={show({
          venues: [
            {
              id: 9,
              name: 'Unlisted Room',
              slug: '',
              city: 'Phoenix',
              state: 'AZ',
              timezone: 'America/Phoenix',
              verified: false,
            },
          ],
        })}
        trailing={null}
      />
    )

    expect(screen.getByText(/Unlisted Room/)).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Unlisted Room' })
    ).not.toBeInTheDocument()
  })

  it('keeps the cancelled mark the card carried', () => {
    render(<ShowRow show={show({ is_cancelled: true })} trailing={null} />)

    expect(screen.getByText('CANCELLED')).toBeInTheDocument()
    expect(screen.getByRole('article').className).toContain('opacity-60')
  })

  it('puts the trailing slot in its own column and adds no save age', () => {
    render(<ShowRow show={show()} trailing={<span>trail</span>} />)

    const row = screen.getByRole('article')
    expect(row.lastElementChild).toHaveTextContent('trail')
    expect(within(row).queryByText(/^saved /)).not.toBeInTheDocument()
  })
})

describe('ShowRowSaveButton', () => {
  it('is an icon-only heart with no count in a 44px target', () => {
    render(
      <ShowRowSaveButton
        showId={7}
        saveData={{ save_count: 3, is_saved: false }}
      />
    )

    expect(saveButtonProps).toHaveBeenCalledWith(
      expect.objectContaining({
        showId: 7,
        showCount: false,
        savedTone: 'primary',
        saveData: { save_count: 3, is_saved: false },
      })
    )
    const props = saveButtonProps.mock.calls[0][0]
    expect(props.showLabel).toBeUndefined()
    expect(props.className).toContain('h-11 w-11')
  })
})
