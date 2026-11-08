import { describe, expectTypeOf, it } from 'vitest'
import type { components } from '@/types/api'
import type { CalendarSyncChoice, CalendarSyncProvider } from './types'

type GeneratedCalendarSync = NonNullable<components['schemas']['CalendarSync']>

/** Every key any state of the hand-written union declares. */
type ChoiceKeys = CalendarSyncChoice extends infer Choice
  ? Choice extends unknown
    ? keyof Choice
    : never
  : never

// CalendarSyncChoice narrows the flat generated schema per state, so its
// states are spelled by hand. These assertions are checked by `tsc`
// (bun run typecheck), not at runtime: regenerating the API types after the
// backend changes its vocabulary breaks the typecheck until the hand-written
// union follows.
describe('CalendarSyncChoice', () => {
  it('names exactly the states the backend accepts', () => {
    expectTypeOf<CalendarSyncChoice['state']>().toEqualTypeOf<GeneratedCalendarSync['state']>()
  })

  // The generated schema is flat with every per-state field optional, so a
  // renamed field would otherwise still satisfy it.
  it('declares exactly the fields the backend document has', () => {
    expectTypeOf<ChoiceKeys>().toEqualTypeOf<keyof GeneratedCalendarSync>()
  })

  it('takes its providers from the generated contract', () => {
    expectTypeOf<CalendarSyncProvider>().toEqualTypeOf<
      NonNullable<GeneratedCalendarSync['providers']>[number]
    >()
  })
})
