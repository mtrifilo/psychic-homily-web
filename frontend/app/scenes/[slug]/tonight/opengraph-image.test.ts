import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSceneDay, renderSceneWeekOgCard } = vi.hoisted(() => ({
  fetchSceneDay: vi.fn(),
  renderSceneWeekOgCard: vi.fn(),
}))

vi.mock('@/features/scenes/sceneDayApi', () => ({ fetchSceneDay }))
vi.mock('@/features/scenes/sceneWeekOgCard', () => ({ renderSceneWeekOgCard }))

import Image from './opengraph-image'

/**
 * The only decision this route owns: which week key to hand the shared
 * renderer. `renderSceneWeekOgCard(slug)` with no key asks for the CURRENT
 * week, which is the wrong week on Monday before 6am, when tonight is still
 * Sunday. Nothing else in the suite covers that hand-off.
 */
describe('tonight opengraph-image', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    renderSceneWeekOgCard.mockResolvedValue(new Response())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the night\'s week, not the current week', async () => {
    fetchSceneDay.mockResolvedValue({ iso_week: '2026-W31' })

    await Image({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(renderSceneWeekOgCard).toHaveBeenCalledWith('phoenix-az', '2026-W31')
  })

  it('falls back to the current week when the night cannot be named', async () => {
    fetchSceneDay.mockResolvedValue(null)

    await Image({ params: Promise.resolve({ slug: 'nowhere-zz' }) })

    expect(renderSceneWeekOgCard).toHaveBeenCalledWith('nowhere-zz', undefined)
  })

  it('ignores an empty or junk iso_week rather than 404ing the card', async () => {
    fetchSceneDay.mockResolvedValue({ iso_week: '' })
    await Image({ params: Promise.resolve({ slug: 'phoenix-az' }) })
    expect(renderSceneWeekOgCard).toHaveBeenCalledWith('phoenix-az', undefined)

    fetchSceneDay.mockResolvedValue({ iso_week: 'not-a-week' })
    await Image({ params: Promise.resolve({ slug: 'phoenix-az' }) })
    expect(renderSceneWeekOgCard).toHaveBeenLastCalledWith('phoenix-az', undefined)
  })
})
