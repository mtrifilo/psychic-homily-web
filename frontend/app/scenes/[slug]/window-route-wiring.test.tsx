import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('next/navigation', () => ({ notFound: vi.fn() }))

const { getSceneDay, getSceneWeek } = vi.hoisted(() => ({
  getSceneDay: vi.fn(),
  getSceneWeek: vi.fn(),
}))

// The two content components are stubbed to NOTHING. This suite is about which
// props the ROUTE hands them, which is the one thing the component suites
// cannot see: they are handed their props by hand.
vi.mock('@/features/scenes/sceneDayPage', () => ({
  getSceneDay,
  SceneDayContent: (): null => null,
  buildSceneDayMetadata: vi.fn(),
}))
vi.mock('@/features/scenes/sceneWeekPage', () => ({
  getSceneWeek,
  SceneWeekContent: (): null => null,
  buildSceneWeekMetadata: vi.fn(),
}))

import TonightPage from './tonight/page'
import WeekPage from './week/page'
import PeriodPage from './[period]/page'

/**
 * Which route is the ROLLING one, asserted at the routes themselves.
 *
 * `isRollingRoute` is the whole discriminator for what these pages may say
 * about now: the title, the active window chip, the prev/next wording, the
 * quiet copy and the share control all key on it. Being a required prop forces
 * every route to pass A value, not the RIGHT one, and a flipped literal here
 * publishes "Tonight in Phoenix" or "This week in Chicago" at a permalink that
 * means one fixed night or week. Nothing else in the suite reads these four
 * call sites.
 */
describe('the window routes name themselves rolling or dated', () => {
  const dayPayload = { slug: 'phoenix-az', date: '2026-07-31' }
  const weekPayload = { slug: 'phoenix-az', iso_week: '2026-W31' }

  beforeEach(() => {
    vi.clearAllMocks()
    getSceneDay.mockResolvedValue(dayPayload)
    getSceneWeek.mockResolvedValue(weekPayload)
  })

  it('/tonight is rolling', async () => {
    const element = await TonightPage({
      params: Promise.resolve({ slug: 'phoenix-az' }),
    })

    expect(element.props.isRollingRoute).toBe(true)
    expect(element.props.data).toBe(dayPayload)
  })

  it('/week is rolling', async () => {
    const element = await WeekPage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(element.props.isRollingRoute).toBe(true)
    expect(element.props.data).toBe(weekPayload)
  })

  it('a dated day permalink is not', async () => {
    const element = await PeriodPage({
      params: Promise.resolve({ slug: 'phoenix-az', period: '2026-07-31' }),
    })

    expect(element.props.isRollingRoute).toBe(false)
    expect(element.props.data).toBe(dayPayload)
  })

  it('a dated week permalink is not', async () => {
    const element = await PeriodPage({
      params: Promise.resolve({ slug: 'phoenix-az', period: '2026-W31' }),
    })

    expect(element.props.isRollingRoute).toBe(false)
    expect(element.props.data).toBe(weekPayload)
  })
})
