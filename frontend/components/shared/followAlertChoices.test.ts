import { describe, it, expect } from 'vitest'
import {
  followAlertChoice,
  followAlertOptions,
  followAlertPendingNote,
  followAlertsPaused,
  followAlertsPausedNote,
  followAlertSummaryFor,
  followAlertUpdateFor,
  isAlertCapableFollowType,
  VENUE_ALERTS_PENDING_NOTE,
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

// PSY-1896 split delivery in two: artist show alerts deliver, venue ones do
// not. This function is the whole mechanism behind that distinction on the
// Library bar, so a regression here silently restores a claim the product
// spent a commit removing.
describe('followAlertPendingNote', () => {
  it('discloses pending delivery for venues, whose alerts have no notifier', () => {
    expect(followAlertPendingNote('venues')).toBe(VENUE_ALERTS_PENDING_NOTE)
  })

  // The regression this exists to catch: artist alerts DO deliver, so an
  // "any day now" line above the Artists tab is the opposite kind of lie.
  it('says nothing for artists, whose alerts already deliver', () => {
    expect(followAlertPendingNote('artists')).toBeNull()
  })

  it('says nothing for follow types with no alert subscription at all', () => {
    for (const entityType of ['labels', 'tags', 'festivals', 'scenes']) {
      expect(followAlertPendingNote(entityType)).toBeNull()
    }
  })

  // A bare object index would return Object.prototype here, which React then
  // throws on rendering. No caller can reach these today; this pins the guard
  // so that stays true if one ever forwards a route segment.
  it('does not leak inherited object properties as a note', () => {
    for (const key of ['__proto__', 'constructor', 'toString', 'hasOwnProperty']) {
      expect(followAlertPendingNote(key)).toBeNull()
    }
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

  // With no home area, "Near me" is not offered, so "Everywhere" is not a
  // choice BETWEEN scopes: it is the only way to say ON, and the read path
  // already relabels a stored near-me as everywhere in that state. Pinning the
  // scope from that click destroys a preference the user never revisited.
  describe('when the viewer has no home area', () => {
    const noArea = { hasHomeMetro: false }

    it('turns alerts on without overwriting a stored near-me scope', () => {
      expect(followAlertUpdateFor('everywhere', noArea)).toEqual({
        shows: { enabled: true },
      })
    })

    // The full round trip the bug produced: a near-me follow toggled off and
    // back on used to come back as everywhere, permanently.
    it('survives an off-then-on round trip with the scope intact', () => {
      expect(followAlertUpdateFor('off', noArea).shows?.scope).toBeUndefined()
      expect(
        followAlertUpdateFor('everywhere', noArea).shows?.scope
      ).toBeUndefined()
    })
  })

  // With an area set, Everywhere IS a deliberate choice between two offered
  // scopes, so it must still pin.
  it('pins everywhere when the viewer has an area to choose against', () => {
    expect(followAlertUpdateFor('everywhere', { hasHomeMetro: true })).toEqual({
      shows: { enabled: true, scope: 'everywhere' },
    })
  })

  // Unknown is not "no area": guessing either way here is what the tri-state
  // exists to prevent, and the surfaces do not offer the control until it
  // resolves anyway.
  it('pins everywhere while the home area is still unknown', () => {
    expect(
      followAlertUpdateFor('everywhere', { hasHomeMetro: undefined })
    ).toEqual({ shows: { enabled: true, scope: 'everywhere' } })
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
          // Only the pairing is pinned: any choice returned ALONGSIDE an
          // option list must be nameable in it. The two functions do NOT agree
          // in every state (an artist with an unknown home area and a disabled
          // subscription yields choice 'off' with no options), which is
          // harmless only because both render surfaces guard on
          // `!current || !options` together. The previous assertion here
          // claimed to pin that agreement and was a tautology inside its own
          // guard, so it could not have failed for any implementation.
          if (!options || !choice) continue
          expect(followAlertSummaryFor(options, choice)).toBeDefined()
        }
      }
    }
  })
})

// PAUSED: enabled, and reaching nobody.
//
// Channels are an ACCOUNT setting, so switching both off silences every follow
// at once while leaving each subscription enabled with its scope intact. These
// cases mirror the notifier's own predicate, which skips the recipient on
// `!pref.Enabled || (!pref.InApp && !pref.Email)`.
describe('followAlertsPaused', () => {
  it('is true for an enabled subscription with no channel behind it', () => {
    expect(
      followAlertsPaused(settings({ enabled: true, in_app: false, email: false }))
    ).toBe(true)
  })

  it('is false while either channel can still carry it', () => {
    expect(
      followAlertsPaused(settings({ enabled: true, in_app: true, email: false }))
    ).toBe(false)
    expect(
      followAlertsPaused(settings({ enabled: true, in_app: false, email: true }))
    ).toBe(false)
  })

  // OFF is a choice made on this follow; PAUSED is an account-wide silence
  // sitting on top of one. Showing "paused" over a subscription the user
  // themselves switched off would send them to fix a channel that is not what
  // is stopping it.
  it('is false for a subscription the user switched off, channels or not', () => {
    expect(
      followAlertsPaused(settings({ enabled: false, in_app: false, email: false }))
    ).toBe(false)
    expect(
      followAlertsPaused(settings({ enabled: false, in_app: true, email: true }))
    ).toBe(false)
  })

  // UNKNOWN is not paused. The read is in flight on every cold render, and
  // reporting a pause there would flash "paused" over a live subscription.
  it('is false while the subscription is still unknown', () => {
    expect(followAlertsPaused(undefined)).toBe(false)
  })
})

describe('followAlertsPausedNote', () => {
  // The cause is NOT knowable from a resolved subscription: a stored
  // per-follow channel override beats the account matrix server-side, so
  // "because they are off in your alert settings" would send an
  // override-paused follow to a card whose boxes read ON.
  it('states the effect without blaming the account matrix for it', () => {
    const note = followAlertsPausedNote('artists')
    expect(note).toContain('nothing is delivered')
    expect(note).not.toMatch(/because/i)
    expect(note).not.toMatch(/switched off .* in your alert settings/i)
  })

  it('tells an artist its stored scope survived', () => {
    expect(followAlertsPausedNote('artists')).toContain(
      'The scope for this follow is saved'
    )
  })

  // A venue has no scope axis at all, so a scope to come back to is a setting
  // that follow never had.
  it('promises a venue no scope, because a venue has none', () => {
    expect(followAlertsPausedNote('venues')).not.toContain('scope for this follow')
  })

  // "Resumes them" is a DELIVERY claim, and a venue follow has no notifier to
  // resume. Lifting the pause is what a channel actually does, and it is true
  // of every follow type.
  it('promises the pause lifted, never delivery resumed', () => {
    for (const entityType of ['artists', 'venues']) {
      const note = followAlertsPausedNote(entityType)
      expect(note).toContain('lifts the pause')
      expect(note).not.toContain('resumes them')
    }
  })

  // Pausing hides the control that used to carry the pending disclosure, and
  // "they resume when you switch a channel back on" is a promise venue
  // delivery cannot keep yet.
  it('keeps the pending-delivery disclosure a paused surface would otherwise drop', () => {
    expect(followAlertsPausedNote('venues')).toContain(
      VENUE_ALERTS_PENDING_NOTE
    )
    expect(followAlertsPausedNote('artists')).not.toContain(
      VENUE_ALERTS_PENDING_NOTE
    )
  })
})

// A paused follow's stored scope is what makes un-pausing restore rather than
// re-ask, so nothing on the paused path may write it. This pins the read half
// of that promise: the choice is still near_me underneath.
describe('a paused subscription keeps its stored choice', () => {
  it('still resolves to near_me, so re-enabling a channel restores it', () => {
    const paused = settings({
      enabled: true,
      in_app: false,
      email: false,
      scope: 'near_me',
    })
    expect(followAlertsPaused(paused)).toBe(true)
    expect(
      followAlertChoice(paused, { entityType: 'artists', hasHomeMetro: true })
    ).toBe('near_me')
  })
})
