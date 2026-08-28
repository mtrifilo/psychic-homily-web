import { describe, it, expect } from 'vitest'
import type { VenueFieldNote } from './types'
import {
  isSetlistSpoiler,
  fieldNoteTeaserText,
  pickFieldNoteForTeaser,
} from './teaser'

describe('isSetlistSpoiler', () => {
  it('is true only for an explicit flag', () => {
    expect(isSetlistSpoiler({ structured_data: { setlist_spoiler: true } })).toBe(
      true,
    )
    expect(
      isSetlistSpoiler({ structured_data: { setlist_spoiler: false } }),
    ).toBe(false)
    expect(isSetlistSpoiler({ structured_data: null })).toBe(false)
    expect(isSetlistSpoiler({})).toBe(false)
  })
})

describe('fieldNoteTeaserText', () => {
  it('strips paired emphasis, code and strikethrough', () => {
    expect(fieldNoteTeaserText('**Loudest** set of the *year*')).toBe(
      'Loudest set of the year',
    )
    expect(fieldNoteTeaserText('__bold__ and _italic_ and `code`')).toBe(
      'bold and italic and code',
    )
    expect(fieldNoteTeaserText('~~not~~ great')).toBe('not great')
  })

  // The hazard that matters: this function's input is mostly ORDINARY PROSE,
  // and an eager emphasis rule DELETES characters from somebody's verbatim
  // words rather than leaving a stray marker. CommonMark does not treat
  // intraword `_` as emphasis either, so honouring it was simply wrong.
  it('leaves intraword underscores and asterisks alone', () => {
    expect(fieldNoteTeaserText('played lo_fi_house all night')).toBe(
      'played lo_fi_house all night',
    )
    expect(
      fieldNoteTeaserText('see https://ex.com/a_b_c for the set'),
    ).toBe('see https://ex.com/a_b_c for the set')
    expect(
      fieldNoteTeaserText('https://bandcamp.com/track/dark_star_live'),
    ).toBe('https://bandcamp.com/track/dark_star_live')
  })

  it('does not swallow separator asterisks between words', () => {
    // "$20 * 2 tickets * for us" must not collapse into "$20 2 tickets for us".
    expect(fieldNoteTeaserText('$20 * 2 tickets * for us')).toBe(
      '$20 * 2 tickets * for us',
    )
  })

  it('keeps link and image TEXT and drops the target', () => {
    expect(fieldNoteTeaserText('see the [setlist](https://example.com/x)')).toBe(
      'see the setlist',
    )
    expect(
      fieldNoteTeaserText('![the room](https://example.com/i.jpg) packed'),
    ).toBe('the room packed')
  })

  it('strips line-leading block markers', () => {
    expect(fieldNoteTeaserText('## Encore')).toBe('Encore')
    expect(fieldNoteTeaserText('> quoted line')).toBe('quoted line')
    expect(fieldNoteTeaserText('- first\n- second')).toBe('first second')
    expect(fieldNoteTeaserText('1. first\n2. second')).toBe('first second')
  })

  it('collapses paragraph breaks into single spaces', () => {
    expect(fieldNoteTeaserText('ended\n\nthen encored')).toBe(
      'ended then encored',
    )
  })

  // `body` is raw author input — only `body_html` is sanitized — so literal
  // HTML must not show its angle brackets. React escapes the text child either
  // way, so this is legibility, not an XSS fix.
  it('flattens literal HTML to its text', () => {
    expect(fieldNoteTeaserText('<b>loudest</b> night')).toBe('loudest night')
    expect(fieldNoteTeaserText('packed <img src=x> room')).toBe('packed room')
    expect(fieldNoteTeaserText('<script>alert(1)</script>')).toBe('alert(1)')
  })

  // Angle brackets as COMPARISONS, which is ordinary prose. An unanchored
  // `/<[^>]*>/` deletes everything between them and publicly misquotes a named
  // author — and disagrees with the canonical renderer, since goldmark does
  // not treat `< 3` as HTML either.
  it('does not eat prose between comparison operators', () => {
    expect(
      fieldNoteTeaserText('2 < 3 but the set > everything else was loud'),
    ).toBe('2 < 3 but the set > everything else was loud')
    expect(
      fieldNoteTeaserText('crowd was < 20 people and the vibe > any big room'),
    ).toBe('crowd was < 20 people and the vibe > any big room')
    expect(fieldNoteTeaserText('set at 9 <-> 11')).toBe('set at 9 <-> 11')
  })

  it('leaves intraword double underscores alone', () => {
    expect(fieldNoteTeaserText('the set was snake__case__weird')).toBe(
      'the set was snake__case__weird',
    )
    expect(fieldNoteTeaserText('DOOM__2024__tour was the shirt')).toBe(
      'DOOM__2024__tour was the shirt',
    )
  })

  it('keeps a link whose target contains parentheses intact', () => {
    expect(
      fieldNoteTeaserText(
        'see [Doom (music)](https://en.wikipedia.org/wiki/Doom_(music)) for context',
      ),
    ).toBe('see Doom (music) for context')
  })

  it('returns empty for a body that is only markup or whitespace', () => {
    expect(fieldNoteTeaserText('   \n\n  ')).toBe('')
    expect(fieldNoteTeaserText('##   ')).toBe('')
  })
})

describe('pickFieldNoteForTeaser', () => {
  function note(overrides: Partial<VenueFieldNote> = {}): VenueFieldNote {
    return {
      id: 1,
      body: 'a real note',
      show_title: 'Doom Night',
      show_artists: [],
      show_date: '2024-06-15T04:00:00Z',
      ...overrides,
    } as VenueFieldNote
  }

  it('takes the first note and hands back its prose', () => {
    const picked = pickFieldNoteForTeaser([
      note({ id: 1, body: '**best**' }),
      note({ id: 2 }),
    ])
    expect(picked?.note.id).toBe(1)
    // The caller renders this rather than re-flattening the body itself.
    expect(picked?.text).toBe('best')
  })

  // Defense in depth: the venue rollup already excludes spoilers server-side,
  // but a teaser fed from any other endpoint must not depend on that.
  it('never quotes a setlist-spoiler note, even ranked first', () => {
    const spoiler = note({ id: 1, structured_data: { setlist_spoiler: true } })
    const safe = note({ id: 2, body: 'no spoilers here' })
    expect(pickFieldNoteForTeaser([spoiler, safe])?.note.id).toBe(2)
    expect(pickFieldNoteForTeaser([spoiler])).toBeNull()
  })

  it('treats a missing or false spoiler flag as quotable', () => {
    expect(
      pickFieldNoteForTeaser([
        note({ structured_data: { setlist_spoiler: false } }),
      ]),
    ).not.toBeNull()
    expect(
      pickFieldNoteForTeaser([note({ structured_data: null })]),
    ).not.toBeNull()
  })

  // The regression the BLOCK finding was about: most shows carry no title, and
  // dropping them would have hidden the teaser on the majority of venues.
  it('KEEPS a note whose show has no title', () => {
    const picked = pickFieldNoteForTeaser([
      note({ show_title: '', show_artists: ['Neckbeard'] }),
    ])
    expect(picked).not.toBeNull()
    expect(picked?.note.show_title).toBe('')
  })

  it('skips a note whose body flattens to nothing', () => {
    const empty = note({ id: 1, body: '   ' })
    const real = note({ id: 2 })
    expect(pickFieldNoteForTeaser([empty, real])?.note.id).toBe(2)
  })

  it('returns null for an absent, empty or wholly unquotable page', () => {
    expect(pickFieldNoteForTeaser(null)).toBeNull()
    expect(pickFieldNoteForTeaser(undefined)).toBeNull()
    expect(pickFieldNoteForTeaser([])).toBeNull()
    expect(
      pickFieldNoteForTeaser([
        note({ structured_data: { setlist_spoiler: true } }),
        note({ body: '' }),
      ]),
    ).toBeNull()
  })
})
