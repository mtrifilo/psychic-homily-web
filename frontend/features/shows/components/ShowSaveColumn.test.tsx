import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, renderHook, screen } from '@testing-library/react'
import type { BatchedSaveData } from '@/components/shared/batchedSaveData'

// The real SaveButton renders here; only its data and navigation edges are
// stubbed, as SaveButton's own suite stubs them.
vi.mock('@/features/shows', () => ({
  useSaveShowToggle: () => ({ isLoading: false, toggle: vi.fn(), error: null }),
  useShowSaveCount: () => ({ data: undefined }),
}))
vi.mock('@/features/shows/hooks/useFirstSaveHint', async importOriginal => ({
  ...(await importOriginal<
    typeof import('@/features/shows/hooks/useFirstSaveHint')
  >()),
  useShouldOpenFirstSaveHint: () => async () => false,
}))
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/artists/calexico',
}))

let viewer: {
  user: { id: string } | null
  authStatus: 'pending' | 'anonymous' | 'authenticated'
} = { user: { id: 'u-7' }, authStatus: 'authenticated' }
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(viewer) }
})

const batchCalls: unknown[][] = []
vi.mock('../hooks/useSavedShows', () => ({
  useShowSaveCountBatch: (...args: unknown[]) => {
    batchCalls.push(args)
    return { data: undefined }
  },
}))

import {
  ShowSaveCell,
  ShowSaveHeaderCell,
  useShowSaveColumn,
} from './ShowSaveColumn'

function renderColumn(saveData: BatchedSaveData | undefined) {
  return render(<Column saveData={saveData} />)
}

function Column({ saveData }: { saveData: BatchedSaveData | undefined }) {
  return (
    <table>
      <thead>
        <tr>
          <ShowSaveHeaderCell />
        </tr>
      </thead>
      <tbody>
        <tr>
          <ShowSaveCell
            showId={7}
            saveData={saveData}
            className="max-sm:col-start-3"
          />
        </tr>
      </tbody>
    </table>
  )
}

/** The fixed widths a cell carries, in order (`w-[..]` and breakpoint forms). */
const widthClasses = (el: Element) =>
  [...el.classList].filter(name => /(^|:)w-\[\d+px\]$/.test(name))

beforeEach(() => {
  viewer = { user: { id: 'u-7' }, authStatus: 'authenticated' }
  batchCalls.length = 0
})

describe('useShowSaveColumn', () => {
  it('asks one batch for the rows, keyed to the signed-in viewer', () => {
    const { result } = renderHook(() =>
      useShowSaveColumn([{ id: 7 }, { id: 8 }])
    )

    expect(batchCalls.at(-1)).toEqual([[7, 8], true, 'u-7'])
    expect(result.current).toEqual({ counts: undefined })
  })

  it('asks the same batch for an anonymous viewer, with no viewer id', () => {
    viewer = { user: null, authStatus: 'anonymous' }
    renderHook(() => useShowSaveColumn([{ id: 7 }]))

    expect(batchCalls.at(-1)).toEqual([[7], false, undefined])
  })
})

describe('ShowSaveCell', () => {
  it('reads Saved with the public count in the name only', () => {
    renderColumn({ save_count: 3, is_saved: true })

    const control = screen.getByRole('button')
    expect(control).toHaveAccessibleName(
      'Saved, remove from saved shows (3 saved)'
    )
    expect(control).toHaveTextContent(/^Saved$/)
  })

  it('reads Save while its batch entry is pending', () => {
    renderColumn('pending')

    const control = screen.getByRole('button')
    expect(control).toHaveAccessibleName('Save show')
    expect(control).toHaveTextContent(/^Save$/)
  })

  it('holds one fixed width on the header and the cell, saved or not', () => {
    const { rerender, container } = renderColumn('pending')
    const header = container.querySelector('th')!
    const cell = container.querySelector('td')!
    const pendingWidths = widthClasses(cell)
    expect(pendingWidths.length).toBeGreaterThan(0)
    // The header sizes the column from `sm` up, where the header row shows.
    expect(widthClasses(header)).toEqual(
      pendingWidths.filter(name => !name.startsWith('max-sm:'))
    )

    rerender(<Column saveData={{ save_count: 3, is_saved: true }} />)

    expect(widthClasses(container.querySelector('td')!)).toEqual(pendingWidths)
    expect(container.querySelector('td')).toHaveClass('max-sm:col-start-3')
  })
})
