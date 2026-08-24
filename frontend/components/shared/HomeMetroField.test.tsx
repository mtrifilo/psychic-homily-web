import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import { HomeMetroSelect, useHomeMetroLabel } from './HomeMetroField'

// The one control that writes user_preferences.home_metro (PSY-1907).

const mockSetHomeMetro = vi.fn()
let mockSetState = { isPending: false, isError: false }
let mockScenesLoading = false
let mockScenes: Array<{ metro: string; name: string }> = []

vi.mock('@/features/charts/hooks/useCharts', () => ({
  useChartScenes: () => ({
    data: { scenes: mockScenes },
    isLoading: mockScenesLoading,
  }),
}))

vi.mock('@/features/auth/hooks/useAlertPreferences', () => ({
  useSetHomeMetro: () => ({ mutate: mockSetHomeMetro, ...mockSetState }),
}))

const SCENES = [
  { metro: '38060', name: 'Phoenix-Mesa-Chandler, AZ' },
  { metro: '16980', name: 'Chicago-Naperville-Elgin, IL' },
]

beforeEach(() => {
  mockScenes = SCENES
})

describe('useHomeMetroLabel', () => {
  it('resolves a stored CBSA code to its display name', () => {
    const { result } = renderHook(() => useHomeMetroLabel('38060'))
    expect(result.current).toBe('Phoenix-Mesa-Chandler, AZ')
  })

  it('is null when no area is set', () => {
    const { result } = renderHook(() => useHomeMetroLabel(null))
    expect(result.current).toBeNull()
  })

  // Falling back to the bare code rather than a guess: a code with no matching
  // scene means we track no shows there, and inventing a name would hide that.
  it('falls back to the raw code for a metro we track no shows in', () => {
    const { result } = renderHook(() => useHomeMetroLabel('99999'))
    expect(result.current).toBe('99999')
  })
})

describe('HomeMetroSelect', () => {
  beforeEach(() => {
    mockSetHomeMetro.mockReset()
    mockSetState = { isPending: false, isError: false }
    mockScenesLoading = false
    mockScenes = SCENES
  })

  // The list is derived from metros we track shows in, so it can legitimately
  // come back empty. A select whose only row is "no home area" reads as broken.
  it('explains an empty metro list instead of rendering a dead select', () => {
    mockScenes = []
    renderWithProviders(<HomeMetroSelect metro={null} />)

    expect(screen.queryByRole('combobox')).toBeNull()
    expect(
      screen.getByText(/No metros are available to choose from yet/)
    ).toBeInTheDocument()
  })

  it('keeps the select mounted while the list is still loading', () => {
    mockScenes = []
    mockScenesLoading = true
    renderWithProviders(<HomeMetroSelect metro={null} />)

    expect(screen.getByRole('combobox', { name: 'Your area' })).toBeDisabled()
  })

  it('shows the stored area as the current value', () => {
    renderWithProviders(<HomeMetroSelect metro="38060" />)
    expect(screen.getByRole('combobox', { name: 'Your area' })).toHaveTextContent(
      'Phoenix-Mesa-Chandler, AZ'
    )
  })

  it('saves a newly chosen metro by its CBSA code', async () => {
    const user = userEvent.setup()
    renderWithProviders(<HomeMetroSelect metro="38060" />)

    await user.click(screen.getByRole('combobox', { name: 'Your area' }))
    await user.click(
      screen.getByRole('option', { name: 'Chicago-Naperville-Elgin, IL' })
    )

    expect(mockSetHomeMetro).toHaveBeenCalledWith('16980', expect.anything())
  })

  // NULL is the state that makes the near-me fallback reachable, so clearing
  // has to be expressible rather than only ever swapping one metro for another.
  it('clears the area with an explicit null', async () => {
    const user = userEvent.setup()
    renderWithProviders(<HomeMetroSelect metro="38060" />)

    await user.click(screen.getByRole('combobox', { name: 'Your area' }))
    await user.click(screen.getByRole('option', { name: 'No home area' }))

    expect(mockSetHomeMetro).toHaveBeenCalledWith(null, expect.anything())
  })

  it('does not re-save the value already stored', async () => {
    const user = userEvent.setup()
    renderWithProviders(<HomeMetroSelect metro="38060" />)

    await user.click(screen.getByRole('combobox', { name: 'Your area' }))
    await user.click(
      screen.getByRole('option', { name: 'Phoenix-Mesa-Chandler, AZ' })
    )

    expect(mockSetHomeMetro).not.toHaveBeenCalled()
  })

  it('parks the control while the write is in flight', () => {
    mockSetState = { isPending: true, isError: false }
    renderWithProviders(<HomeMetroSelect metro="38060" />)
    expect(screen.getByRole('combobox', { name: 'Your area' })).toBeDisabled()
  })

  it('surfaces a rejected metro rather than looking saved', () => {
    mockSetState = { isPending: false, isError: true }
    renderWithProviders(<HomeMetroSelect metro="38060" />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      "Couldn't save your area. Try again."
    )
  })

  it('takes an accessible-name override for the second call site', () => {
    renderWithProviders(<HomeMetroSelect metro={null} ariaLabel="Home area" />)
    expect(screen.getByRole('combobox', { name: 'Home area' })).toBeInTheDocument()
  })
})
