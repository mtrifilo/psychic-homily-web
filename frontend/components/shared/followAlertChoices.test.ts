import { describe, it, expect } from 'vitest'
import {
  followAlertChoice,
  followAlertOptions,
  followAlertSummary,
  followAlertUpdateFor,
  isAlertCapableFollowType,
} from './followAlertChoices'
import type { FollowAlertSettings } from '@/lib/types/follow'

// The single vocabulary two surfaces share (PSY-1905). These cases are the
// contract between the entity-page reveal and the Library row menu.

const settings = (
  shows: Partial<FollowAlertSettings['shows']>
): FollowAlertSettings => ({
  entity_type: 'artist',
  entity_id: 1,
  shows: { enabled: true, in_app: true, email: false, ...shows },
})

describe('isAlertCapableFollowType', () => {
  it('accepts the two follow types whose follow carries a subscription', () => {
    expect(isAlertCapableFollowType('artists')).toBe(true)
    expect(isAlertCapableFollowType('venues')).toBe(true)
  })

  // These 422 on the alert endpoints, so offering the control would be a lie.
  it('rejects every follow type that has no alert subscription', () => {
    for (const type of ['labels', 'festivals', 'tags', 'scenes', 'radio-shows']) {
      expect(isAlertCapableFollowType(type)).toBe(false)
    }
  })
})

describe('followAlertOptions', () => {
  it('gives a venue only on and off, because a venue sits in one place', () => {
    expect(
      followAlertOptions({ entityType: 'venues', hasHomeMetro: true }).map(
        o => o.value
      )
    ).toEqual(['on', 'off'])
  })

  it('gives an artist a geographic scope when a home area exists', () => {
    expect(
      followAlertOptions({ entityType: 'artists', hasHomeMetro: true }).map(
        o => o.value
      )
    ).toEqual(['near_me', 'everywhere', 'off'])
  })

  // A scoped subscription with nothing to scope to would look configured and
  // deliver nothing, which is exactly the failure this withholding removes.
  it('withholds near me until a home area exists', () => {
    expect(
      followAlertOptions({ entityType: 'artists', hasHomeMetro: false }).map(
        o => o.value
      )
    ).toEqual(['everywhere', 'off'])
  })

  it('offers a venue on/off regardless of the home area', () => {
    expect(
      followAlertOptions({ entityType: 'venues', hasHomeMetro: false }).map(
        o => o.value
      )
    ).toEqual(['on', 'off'])
  })
})

describe('followAlertChoice', () => {
  const artist = { entityType: 'artists', hasHomeMetro: true }

  it('is undefined until the resolved subscription lands', () => {
    expect(followAlertChoice(undefined, artist)).toBeUndefined()
  })

  it('reads a disabled subscription as off, whatever the scope says', () => {
    expect(
      followAlertChoice(settings({ enabled: false, scope: 'near_me' }), artist)
    ).toBe('off')
  })

  it('reads an enabled venue follow as on', () => {
    expect(
      followAlertChoice(settings({ enabled: true }), {
        entityType: 'venues',
        hasHomeMetro: true,
      })
    ).toBe('on')
  })

  it('reads the stored artist scope', () => {
    expect(followAlertChoice(settings({ scope: 'near_me' }), artist)).toBe(
      'near_me'
    )
    expect(followAlertChoice(settings({ scope: 'everywhere' }), artist)).toBe(
      'everywhere'
    )
  })

  // The near-me fallback is applied at delivery time rather than baked into
  // storage, so the display has to apply it too or it would promise a reach
  // the server will not honour.
  it('shows near me as everywhere when no home area backs it', () => {
    expect(
      followAlertChoice(settings({ scope: 'near_me' }), {
        entityType: 'artists',
        hasHomeMetro: false,
      })
    ).toBe('everywhere')
  })

  it('treats a missing scope as everywhere', () => {
    expect(followAlertChoice(settings({}), artist)).toBe('everywhere')
  })
})

describe('followAlertUpdateFor', () => {
  // Each choice pins only the axes it decides. Sending a scope alongside an
  // "off" would store a preference the user never expressed.
  it('turns alerts off without touching the scope', () => {
    expect(followAlertUpdateFor('off')).toEqual({ shows: { enabled: false } })
  })

  it('turns a venue on without inventing a scope it has no axis for', () => {
    expect(followAlertUpdateFor('on')).toEqual({ shows: { enabled: true } })
  })

  it('pins enabled and scope together for an artist scope choice', () => {
    expect(followAlertUpdateFor('near_me')).toEqual({
      shows: { enabled: true, scope: 'near_me' },
    })
    expect(followAlertUpdateFor('everywhere')).toEqual({
      shows: { enabled: true, scope: 'everywhere' },
    })
  })

  it('never writes the releases axis, which has no geography', () => {
    for (const choice of ['off', 'on', 'near_me', 'everywhere'] as const) {
      expect(followAlertUpdateFor(choice).releases).toBeUndefined()
    }
  })
})

describe('followAlertSummary', () => {
  it('renders the lower-case bracket text for each choice', () => {
    expect(
      followAlertSummary(settings({ scope: 'near_me' }), {
        entityType: 'artists',
        hasHomeMetro: true,
      })
    ).toBe('near me')
    expect(
      followAlertSummary(settings({ enabled: false }), {
        entityType: 'artists',
        hasHomeMetro: true,
      })
    ).toBe('off')
    expect(
      followAlertSummary(settings({ enabled: true }), {
        entityType: 'venues',
        hasHomeMetro: true,
      })
    ).toBe('on')
  })

  it('is undefined with no subscription to summarize', () => {
    expect(
      followAlertSummary(undefined, { entityType: 'artists', hasHomeMetro: true })
    ).toBeUndefined()
  })
})
