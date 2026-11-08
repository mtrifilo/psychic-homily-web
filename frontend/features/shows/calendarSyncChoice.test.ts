import { describe, expectTypeOf, it } from 'vitest'
import type { components } from '@/types/api'
import type { CalendarSyncChoice, CalendarSyncProvider } from './types'

type GeneratedCalendarSync = NonNullable<components['schemas']['CalendarSync']>

// CalendarSyncChoice narrows the flat generated schema per state, so its
// states are spelled by hand. These assertions are checked by `tsc`
// (bun run typecheck), not at runtime: regenerating the API types after the
// backend changes its vocabulary breaks the typecheck until the hand-written
// union follows.
describe('CalendarSyncChoice', () => {
  it('names exactly the states the backend accepts', () => {
    expectTypeOf<CalendarSyncChoice['state']>().toEqualTypeOf<GeneratedCalendarSync['state']>()
  })

  it('takes its providers from the generated contract', () => {
    expectTypeOf<CalendarSyncProvider>().toEqualTypeOf<
      NonNullable<GeneratedCalendarSync['providers']>[number]
    >()
  })
})
