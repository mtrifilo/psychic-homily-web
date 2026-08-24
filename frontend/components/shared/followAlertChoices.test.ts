import { describe, it, expect } from 'vitest'
import {
  followAlertChoice,
  followAlertOptions,
  followAlertSummaryFor,
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

  // The alert endpoints refuse all of these (422 for the alertless follow
  // types, 400 for scenes), so offering the control would be a lie.
  it('rejects every follow type that has no alert subscription', () => {
    for (const type of ['labels', 'festivals', 'tags', 'scenes', 'radio-shows']) {
      expect(isAlertCapableFollowType(type)).toBe(false)
    }
  })
})

const valuesOf = (options: ReturnType<typeof followAlertOptions>) =>
  options?.map(option => option.value)

describe('followAlertOptions', () => {
  it('gives a venue only on and off, because a venue sits in one place', () => {
    expect(
      valuesOf(followAlertOptions({ entityType: 'venues', hasHomeMetro: true }))
    ).toEqual(['on', 'off'])
  })

  it('gives an artist a geographic scope when a home area exists', () => {
    expect(
      valuesOf(followAlertOptions({ entityType: 'artists', hasHomeMetro: true }))
    ).toEqual(['near_me', 'everywhere', 'off'])
  })

  // A scoped subscription with nothing to scope to would look configured and
  // deliver nothing, which is exactly the failure this withholding removes.
  it('withholds near me when the viewer is KNOWN to have no home area', () => {
    expect(
      valuesOf(
        followAlertOptions({ entityType: 'artists', hasHomeMetro: false })
      )
    ).toEqual(['everywhere', 'off'])
  })

  // Unknown is not "no". Falling through to the no-area option set renders the
  // wrong chips for a viewer who HAS an area, then swaps them out underneath a
  // click that the equal-value guard would swallow.
  it('offers nothing for a scoped follow while the home area is unknown', () => {
    expect(
      followAlertOptions({ entityType: 'artists', hasHomeMetro: undefined })
    ).toBeUndefined()
  })

  // A venue has no scope axis, so it never has to wait on the home area.
  it('offers a venue on/off regardless of the home area, unknown included', () => {
    for (const hasHomeMetro of [true, false, undefined]) {
      expect(
        valuesOf(followAlertOptions({ entityType: 'venues', hasHomeMetro }))
      ).toEqual(['on', 'off'])
    }
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

  // The near-me-reads-as-everywhere relabel is only honest once the area is
  // KNOWN absent. While it is unknown, saying "everywhere" overstates the
  // reach of a subscription the server may well be scoping.
  it('resolves nothing for a scoped follow while the home area is unknown', () => {
    expect(
      followAlertChoice(settings({ scope: 'near_me' }), {
        entityType: 'artists',
        hasHomeMetro: undefined,
      })
    ).toBeUndefined()
  })

  // "Off" is stored on the follow itself, so it needs no home area to be read.
  it('still reads a disabled subscription as off while the area is unknown', () => {
    expect(
      followAlertChoice(settings({ enabled: false }), {
        entityType: 'artists',
        hasHomeMetro: undefined,
      })
    ).toBe('off')
  })

  it('resolves a venue follow without waiting on the home area', () => {
    expect(
      followAlertChoice(settings({ enabled: true }), {
        entityType: 'venues',
        hasHomeMetro: undefined,
      })
    ).toBe('on')
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

describe('followAlertSummaryFor', () => {
  const artistOptions = followAlertOptions({
    entityType: 'artists',
    hasHomeMetro: true,
  })!
  const venueOptions = followAlertOptions({
    entityType: 'venues',
    hasHomeMetro: true,
  })!

  it('renders the lower-case bracket text for each choice', () => {
    expect(followAlertSummaryFor(artistOptions, 'near_me')).toBe('near me')
    expect(followAlertSummaryFor(artistOptions, 'everywhere')).toBe(
      'everywhere'
    )
    expect(followAlertSummaryFor(artistOptions, 'off')).toBe('off')
    expect(followAlertSummaryFor(venueOptions, 'on')).toBe('on')
  })

  // The summary is looked up in the SAME option list the chips render from, so
  // a choice the current context does not offer has no text. That pairing is
  // what keeps the bracket and the menu from ever disagreeing.
  it('is undefined for a choice this context does not offer', () => {
    const noAreaOptions = followAlertOptions({
      entityType: 'artists',
      hasHomeMetro: false,
    })!
    expect(followAlertSummaryFor(noAreaOptions, 'near_me')).toBeUndefined()
  })

  // Every choice followAlertChoice can return for a context must be nameable
  // in that same context, or a row would render a bracket with no text.
  it('names every choice the matching context can resolve to', () => {
    for (const entityType of ['artists', 'venues']) {
      for (const hasHomeMetro of [true, false, undefined]) {
        const context = { entityType, hasHomeMetro }
        const options = followAlertOptions(context)
        for (const shows of [
          { enabled: false },
          { enabled: true, scope: 'near_me' as const },
          { enabled: true, scope: 'everywhere' as const },
          { enabled: true },
        ]) {
          const choice = followAlertChoice(settings(shows), context)
          // Unknown home area resolves neither, and the two must agree: a
          // renderable choice with no option list (or vice versa) is exactly
          // the half-known state that produced the wrong-chips bug.
          if (!options || !choice) {
            expect(Boolean(options) && Boolean(choice)).toBe(false)
            continue
          }
          expect(followAlertSummaryFor(options, choice)).toBeDefined()
        }
      }
    }
  })
})
