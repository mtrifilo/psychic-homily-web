import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HomeSectionList } from './HomeSectionList'
import {
  HOME_SECTIONS,
  resolveHomeLayout,
  setHomeSectionVisibility,
  toHomeLayoutDocument,
  type HomeLayoutDocument,
} from '../sections'

const persist = vi.fn()
const reset = vi.fn()
let layout = resolveHomeLayout(null)
let hasError = false

vi.mock('../hooks/useHomeLayout', () => ({
  useHomeLayout: () => layout,
  usePersistHomeLayout: () => ({
    persist,
    reset,
    isResetting: false,
    hasError,
  }),
}))

/** The list writes into the profile cache in production; here the mocked
 *  persist stands in for it, so the next render has to be driven by hand. */
function applyDocument(document: HomeLayoutDocument) {
  layout = resolveHomeLayout(document)
}

beforeEach(() => {
  layout = resolveHomeLayout(null)
  hasError = false
  persist.mockReset()
  reset.mockReset()
})

describe('HomeSectionList', () => {
  it('lists every registry section in order with its copy', () => {
    render(<HomeSectionList />)

    const rows = screen.getAllByRole('listitem')
    expect(rows).toHaveLength(HOME_SECTIONS.length)
    HOME_SECTIONS.forEach((section, index) => {
      expect(within(rows[index]).getByText(section.title)).toBeInTheDocument()
      expect(
        within(rows[index]).getByText(section.description)
      ).toBeInTheDocument()
    })
  })

  it('renders the end buttons disabled rather than removing them', () => {
    render(<HomeSectionList />)

    expect(
      screen.getByRole('button', { name: 'Move Your upcoming shows up' })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Move Your upcoming shows down' })
    ).toBeEnabled()
    expect(
      screen.getByRole('button', { name: 'Move Latest radio shows down' })
    ).toBeDisabled()
  })

  it('persists the swapped order on one click of the up button', async () => {
    const user = userEvent.setup()
    render(<HomeSectionList />)

    await user.click(
      screen.getByRole('button', { name: 'Move Community stats up' })
    )

    expect(persist).toHaveBeenCalledTimes(1)
    expect(persist).toHaveBeenCalledWith({
      version: 1,
      sections: [
        { id: 'saved_shows', visible: true },
        { id: 'community_stats', visible: true },
        { id: 'nearby_shows', visible: true },
        { id: 'city_graph', visible: true },
        { id: 'radio_shows', visible: true },
      ],
    })
  })

  it('announces the move once, at the list level, with the new position', async () => {
    const user = userEvent.setup()
    render(<HomeSectionList />)

    await user.click(
      screen.getByRole('button', { name: 'Move Community stats up' })
    )

    const regions = screen.getAllByRole('status')
    expect(regions).toHaveLength(1)
    expect(regions[0]).toHaveTextContent(
      'Community stats moved up, now 2 of 5.'
    )
  })

  it('keeps focus on the button that was clicked, on the row that moved', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<HomeSectionList />)

    const up = screen.getByRole('button', { name: 'Move Community stats up' })
    await user.click(up)
    applyDocument(persist.mock.calls[0][0])
    rerender(<HomeSectionList />)

    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Move Community stats up' })
      ).toHaveFocus()
    })
  })

  it('hands focus to the sibling when the clicked button reaches the end', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<HomeSectionList />)

    await user.click(
      screen.getByRole('button', { name: 'Move Shows near you this week up' })
    )
    applyDocument(persist.mock.calls[0][0])
    rerender(<HomeSectionList />)

    // Now first in the list: its own ▲ is disabled, so focus lands on ▼.
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Move Shows near you this week down' })
      ).toHaveFocus()
    })
  })

  it('does nothing at all when the row is already at that end', async () => {
    const user = userEvent.setup()
    render(<HomeSectionList />)

    await user.click(
      screen.getByRole('button', { name: 'Move Your upcoming shows up' })
    )

    expect(persist).not.toHaveBeenCalled()
    expect(screen.getByRole('status')).toHaveTextContent('')
  })

  it('persists a hide without reordering, and marks the row hidden', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<HomeSectionList />)

    await user.click(screen.getByRole('checkbox', { name: 'Community stats' }))

    expect(persist).toHaveBeenCalledWith(
      toHomeLayoutDocument(
        setHomeSectionVisibility(resolveHomeLayout(null), 'community_stats', false)
      )
    )
    expect(screen.getByRole('status')).toHaveTextContent('Community stats hidden')

    applyDocument(persist.mock.calls[0][0])
    rerender(<HomeSectionList />)

    const rows = screen.getAllByRole('listitem')
    expect(within(rows[2]).getByText('hidden')).toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'Community stats' })
    ).not.toBeChecked()
  })

  it('announces a re-show', async () => {
    const user = userEvent.setup()
    layout = setHomeSectionVisibility(
      resolveHomeLayout(null),
      'community_stats',
      false
    )
    render(<HomeSectionList />)

    await user.click(screen.getByRole('checkbox', { name: 'Community stats' }))

    expect(screen.getByRole('status')).toHaveTextContent('Community stats shown')
  })

  it('disables reset on the shipped layout and enables it after a change', () => {
    const { rerender } = render(<HomeSectionList />)
    expect(screen.getByRole('button', { name: 'Reset to default' })).toBeDisabled()

    layout = setHomeSectionVisibility(layout, 'radio_shows', false)
    rerender(<HomeSectionList />)
    expect(screen.getByRole('button', { name: 'Reset to default' })).toBeEnabled()
  })

  it('resets through the DELETE path, not a PUT of the default document', async () => {
    const user = userEvent.setup()
    layout = setHomeSectionVisibility(resolveHomeLayout(null), 'radio_shows', false)
    render(<HomeSectionList />)

    await user.click(screen.getByRole('button', { name: 'Reset to default' }))

    expect(reset).toHaveBeenCalledTimes(1)
    expect(persist).not.toHaveBeenCalled()
  })

  it('shows one inline line when a write fails', () => {
    hasError = true
    render(<HomeSectionList />)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not save your layout. Try again.'
    )
  })

  it('tells the page whether a section is about to show or hide', async () => {
    const user = userEvent.setup()
    const onBeforeChange = vi.fn()
    render(<HomeSectionList onBeforeChange={onBeforeChange} />)

    await user.click(screen.getByRole('checkbox', { name: 'Latest radio shows' }))
    expect(onBeforeChange).toHaveBeenLastCalledWith({
      id: 'radio_shows',
      visible: false,
    })

    await user.click(
      screen.getByRole('button', { name: 'Move Latest radio shows up' })
    )
    // Null: a reorder does not transition any one section's height.
    expect(onBeforeChange).toHaveBeenLastCalledWith(null)
  })
})
