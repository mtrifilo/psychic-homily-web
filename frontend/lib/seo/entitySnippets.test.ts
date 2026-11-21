import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  DESCRIPTION_BUDGET,
  SITE_TITLE_SUFFIX,
  TITLE_BUDGET,
  artistNextShowFrom,
  artistSnippet,
  festivalDateRange,
  festivalDescription,
  fitDescription,
  fitTitle,
  labelDescription,
  releaseArtistPhrase,
  releaseSnippet,
  showEventDescription,
  showSnippet,
  type ShowEventDescriptionInput,
  snippetMonthDay,
  snippetPlace,
  statedUpcomingCount,
  venueNextShowFrom,
  venueSnippet,
  withoutEndedShows,
} from './entitySnippets'
import {
  doorsMusicFactSegment,
  startTimeFactSegment,
} from '@/features/shows/components/showStatusStripeCopy'

function titleLength(title: string): number {
  return Array.from(title + SITE_TITLE_SUFFIX).length
}

// 8:00 PM venue-local on each date, as the Product Designs board renders them.
const NIROSTA_AT_HIDEOUT = {
  headliner: 'Nirosta Steel',
  venue: 'Hideout',
  city: 'Chicago',
  state: 'IL',
  timing: {
    eventDate: '2026-09-20T01:00:00Z',
    state: 'IL',
    timezone: 'America/Chicago',
  },
}

const AFTERBIRTH_AT_YUCCA = {
  headliner: 'Afterbirth Cartoons',
  venue: 'Yucca Tap Room',
  city: 'Tempe',
  state: 'AZ',
  timing: {
    eventDate: '2026-09-12T03:00:00Z',
    state: 'AZ',
    timezone: 'America/Phoenix',
  },
}

describe('SITE_TITLE_SUFFIX', () => {
  it('is the suffix the root layout title template appends', () => {
    // Read as source: importing the layout module pulls in next/font, which
    // does not load under Vitest.
    const layout = readFileSync(
      resolve(__dirname, '../../app/layout.tsx'),
      'utf8'
    )
    expect(layout).toContain(`template: '%s${SITE_TITLE_SUFFIX}'`)
  })
})

describe('snippetPlace', () => {
  it('joins city and state', () => {
    expect(snippetPlace('Chicago', 'IL')).toBe('Chicago, IL')
  })

  it('keeps whichever half is present', () => {
    expect(snippetPlace('London', null)).toBe('London')
    expect(snippetPlace(null, 'AZ')).toBe('AZ')
    expect(snippetPlace('  Tempe ', '')).toBe('Tempe')
  })

  it('is null when neither half is present', () => {
    expect(snippetPlace(null, undefined)).toBeNull()
    expect(snippetPlace(' ', '')).toBeNull()
  })
})

describe('fitTitle', () => {
  it('keeps every segment when the full title fits', () => {
    expect(fitTitle('A at B', [', C', ' · D'])).toBe('A at B, C · D')
  })

  it('drops segments from the end first', () => {
    // 40 + ', City' (6) + ' · Sep 9' (8) + suffix (17) = 71; without the date, 63;
    // without both, 57.
    const base = 'x'.repeat(40)
    expect(fitTitle(base, [', City', ' · Sep 9'])).toBe(base)
    const shorter = 'x'.repeat(36)
    expect(fitTitle(shorter, [', City', ' · Sep 9'])).toBe(`${shorter}, City`)
  })

  it('returns the bare base when even the base is over budget', () => {
    const base = 'y'.repeat(80)
    expect(fitTitle(base, [', City'])).toBe(base)
  })

  it('skips absent segments', () => {
    expect(fitTitle('A', [null, ' · D'])).toBe('A · D')
  })

  it('counts code points, not UTF-16 units', () => {
    // 21 emoji are 42 UTF-16 units but 21 characters: 21 + 22 + 17 = 60 fits.
    const base = '🎸'.repeat(21)
    const segment = ', ' + 'z'.repeat(20)
    expect(fitTitle(base, [segment])).toBe(base + segment)
  })
})

describe('fitDescription', () => {
  it('returns text within budget unchanged', () => {
    const exact = 'y'.repeat(DESCRIPTION_BUDGET)
    expect(fitDescription(exact)).toBe(exact)
  })

  it('cuts over-budget text to the budget with the ellipsis inside it', () => {
    const result = fitDescription('x'.repeat(300))
    expect(result).toBe('x'.repeat(DESCRIPTION_BUDGET - 3) + '...')
    expect(result).toHaveLength(DESCRIPTION_BUDGET)
  })

  it('collapses whitespace runs, including newlines', () => {
    expect(fitDescription('  one\n\ntwo \t three ')).toBe('one two three')
  })

  it('does not leave a space before the ellipsis', () => {
    const text = 'x'.repeat(DESCRIPTION_BUDGET - 4) + ' tail that runs over'
    const result = fitDescription(text)
    expect(result).toBe('x'.repeat(DESCRIPTION_BUDGET - 4) + '...')
  })

  it('does not put the ellipsis after a full stop or a comma', () => {
    const endsInStop = 'x'.repeat(DESCRIPTION_BUDGET - 4) + '. More text follows'
    expect(fitDescription(endsInStop)).toBe(
      'x'.repeat(DESCRIPTION_BUDGET - 4) + '...'
    )
    const endsInComma = 'y'.repeat(DESCRIPTION_BUDGET - 5) + ', and more text'
    expect(fitDescription(endsInComma)).toBe(
      'y'.repeat(DESCRIPTION_BUDGET - 5) + '...'
    )
  })

  it('never splits a surrogate pair', () => {
    const result = fitDescription('🎸'.repeat(200))
    expect(Array.from(result)).toHaveLength(DESCRIPTION_BUDGET)
    expect(result.endsWith('🎸...')).toBe(true)
  })
})

describe('snippetMonthDay', () => {
  it('reads the day on the venue calendar', () => {
    // 03:00Z is still the previous evening in Phoenix.
    expect(snippetMonthDay(AFTERBIRTH_AT_YUCCA.timing)).toBe('Sep 11')
  })

  it('marks a day decided by the fallback zone', () => {
    expect(
      snippetMonthDay({ eventDate: '2026-09-12T03:00:00Z', state: 'Ontario' })
    ).toBe('~Sep 11')
  })

  it('is null for a missing or unparseable instant', () => {
    expect(snippetMonthDay({ eventDate: null })).toBeNull()
    expect(snippetMonthDay({ eventDate: 'not-a-date' })).toBeNull()
  })
})

describe('showSnippet', () => {
  it('renders the board row 1a strings', () => {
    const { title, description } = showSnippet(NIROSTA_AT_HIDEOUT)
    expect(title).toBe('Nirosta Steel at Hideout, Chicago · Sep 19')
    expect(titleLength(title)).toBe(59)
    expect(description).toBe(
      'Nirosta Steel live at Hideout in Chicago, IL on Saturday, September 19, 2026.'
    )
  })

  it('drops the date and then the city when the title is over budget (row 1b)', () => {
    const { title, description } = showSnippet(AFTERBIRTH_AT_YUCCA)
    // With the date: 70. Without the date: 61. Without the city too: 54.
    expect(title).toBe('Afterbirth Cartoons at Yucca Tap Room')
    expect(description).toBe(
      'Afterbirth Cartoons live at Yucca Tap Room in Tempe, AZ on Friday, September 11, 2026.'
    )
  })

  it('drops only the date when that is enough', () => {
    // With the date: 64. Without it: 55.
    const { title } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      headliner: 'Nirosta Steel Trio',
    })
    expect(title).toBe('Nirosta Steel Trio at Hideout, Chicago')
  })

  it('keeps the city when the title without the date lands exactly on budget', () => {
    // Without the date: 43 + 17 = 60.
    const { title } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      headliner: 'Nirosta Steel Orchestra',
    })
    expect(title).toBe('Nirosta Steel Orchestra at Hideout, Chicago')
    expect(titleLength(title)).toBe(TITLE_BUDGET)
  })

  it('prefixes an authored description and cuts to the budget (row 4)', () => {
    const { title, description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      authoredDescription:
        'An evening of dream-folk and experimental sounds with a five-act bill spanning headliner through host. Doors at 7, music at 8.',
    })
    expect(title).toBe('Nirosta Steel at Hideout, Chicago · Sep 19')
    expect(description).toBe(
      'Nirosta Steel live at Hideout in Chicago, IL on Saturday, September 19, 2026. An evening of dream-folk and experimental sounds with a five-act bill span...'
    )
    expect(description).toHaveLength(DESCRIPTION_BUDGET)
  })

  it('keeps a short authored description whole after the generated sentence', () => {
    const { description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      authoredDescription: '  All ages.\n\nDoors at 7. ',
    })
    expect(description).toBe(
      'Nirosta Steel live at Hideout in Chicago, IL on Saturday, September 19, 2026. All ages. Doors at 7.'
    )
  })

  it('ignores a blank authored description', () => {
    const { description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      authoredDescription: '   ',
    })
    expect(description).toBe(
      'Nirosta Steel live at Hideout in Chicago, IL on Saturday, September 19, 2026.'
    )
  })

  it('omits the city from the title and the place from the description when there is no location', () => {
    const { title, description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      city: null,
      state: null,
    })
    expect(title).toBe('Nirosta Steel at Hideout · Sep 19')
    expect(description).toBe(
      'Nirosta Steel live at Hideout on Saturday, September 19, 2026.'
    )
  })

  it('names the state alone when there is no city', () => {
    const { title, description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      city: '',
    })
    expect(title).toBe('Nirosta Steel at Hideout · Sep 19')
    expect(description).toContain('live at Hideout in IL on')
  })

  it('omits the date from both strings when the instant is unparseable', () => {
    const { title, description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      timing: { eventDate: 'garbage', state: 'IL', timezone: 'America/Chicago' },
    })
    expect(title).toBe('Nirosta Steel at Hideout, Chicago')
    expect(description).toBe('Nirosta Steel live at Hideout in Chicago, IL.')
  })

  it('marks a guessed day in both the title and the description', () => {
    const { title, description } = showSnippet({
      headliner: 'Band',
      venue: 'Room',
      city: 'Toronto',
      state: 'ON',
      timing: { eventDate: '2026-09-12T03:00:00Z', state: 'ON' },
    })
    expect(title).toBe('Band at Room, Toronto · ~Sep 11')
    expect(description).toBe(
      'Band live at Room in Toronto, ON on ~Friday, September 11, 2026.'
    )
  })

  it('does not double the full stop after a name that ends in one', () => {
    const { description } = showSnippet({
      headliner: 'Band',
      venue: 'Room Inc.',
      timing: { eventDate: null },
    })
    expect(description).toBe('Band live at Room Inc.')
  })

  it('stays within both budgets for long names', () => {
    const { title, description } = showSnippet({
      ...NIROSTA_AT_HIDEOUT,
      headliner: 'The Extraordinarily Long-Named Headlining Ensemble',
      venue: 'The Very Long Name Memorial Auditorium',
      authoredDescription: 'z '.repeat(200),
    })
    // The base itself is over budget, so it is kept whole and bare.
    expect(title).toBe(
      'The Extraordinarily Long-Named Headlining Ensemble at The Very Long Name Memorial Auditorium'
    )
    expect(Array.from(description).length).toBeLessThanOrEqual(DESCRIPTION_BUDGET)
    expect(description.endsWith('...')).toBe(true)
  })

  it('keeps every generated title with a short base inside the title budget', () => {
    for (const headliner of ['A', 'Mid Length Band', 'x'.repeat(30)]) {
      const { title } = showSnippet({ ...NIROSTA_AT_HIDEOUT, headliner })
      expect(titleLength(title)).toBeLessThanOrEqual(TITLE_BUDGET)
    }
  })

  it('has no em dash in any output', () => {
    const { title, description } = showSnippet(NIROSTA_AT_HIDEOUT)
    expect(title + description).not.toContain('\u2014')
  })
})

describe('showEventDescription', () => {
  // Launchpad is in Albuquerque (America/Denver, UTC-7 in November): 02:00Z
  // and 03:00Z on Nov 22 are 7:00 PM and 8:00 PM on Saturday Nov 21.
  const DOORS = '2026-11-22T02:00:00Z'
  const MUSIC = '2026-11-22T03:00:00Z'
  const holyWave = {
    artists: [{ name: 'Holy Wave', is_headliner: true }],
    venue: 'Launchpad',
    city: 'Albuquerque',
    eventDate: MUSIC,
    state: 'NM',
    timezone: 'America/Denver',
  }

  describe('the examples the owner approved', () => {
    it('names the bill, the place, and both announced times', () => {
      expect(
        showEventDescription({
          ...holyWave,
          artists: [
            { name: 'Holy Wave', is_headliner: true },
            { name: 'Support Act', is_headliner: false },
          ],
          doorsAt: DOORS,
          musicAt: MUSIC,
        })
      ).toBe('Holy Wave with Support Act at Launchpad, Albuquerque. Doors 7:00 PM, music 8:00 PM.')
    })

    it('states a lone music time that is the start time the page prints', () => {
      expect(showEventDescription({ ...holyWave, musicAt: MUSIC })).toBe(
        'Holy Wave at Launchpad, Albuquerque. Music 8:00 PM.'
      )
    })

    it('states no time when none was announced', () => {
      expect(showEventDescription(holyWave)).toBe('Holy Wave at Launchpad, Albuquerque.')
    })
  })

  it('lists several support acts comma separated, in bill order', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: 'Holy Wave', is_headliner: true },
          { name: 'First Support' },
          { name: 'Second Support' },
          { name: 'Third Support' },
        ],
      })
    ).toBe('Holy Wave with First Support, Second Support, Third Support at Launchpad, Albuquerque.')
  })

  it('leads with the curated headliner wherever the bill lists it', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: 'Opener', set_type: 'opener' },
          { name: 'Holy Wave', set_type: 'headliner' },
        ],
      })
    ).toBe('Holy Wave with Opener at Launchpad, Albuquerque.')
  })

  it('lists co-headliners together in the lead', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: 'Holy Wave', is_headliner: true },
          { name: 'Co Headliner', is_headliner: true },
          { name: 'Support Act' },
        ],
      })
    ).toBe('Holy Wave, Co Headliner with Support Act at Launchpad, Albuquerque.')
  })

  it('leads with the first act of a bill that names no headliner', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [{ name: 'Holy Wave' }, { name: 'Support Act' }],
      })
    ).toBe('Holy Wave with Support Act at Launchpad, Albuquerque.')
  })

  it('names an act listed twice once, in its first support slot', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: 'Holy Wave', is_headliner: true },
          { name: 'Support Act' },
          { name: 'holy wave ' },
          { name: 'Support Act' },
        ],
      })
    ).toBe('Holy Wave with Support Act at Launchpad, Albuquerque.')
  })

  it('keeps the lead slot for a name that both leads and appears in support', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: 'Holy Wave' },
          { name: 'Opener' },
          { name: 'Holy Wave', set_type: 'headliner' },
        ],
      })
    ).toBe('Holy Wave with Opener at Launchpad, Albuquerque.')
  })

  it('skips blank support names', () => {
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: 'Holy Wave', is_headliner: true },
          { name: '  ' },
          { name: 'Support Act' },
        ],
      })
    ).toBe('Holy Wave with Support Act at Launchpad, Albuquerque.')
  })

  it('names no lead when the act the header leads with has a blank name', () => {
    // The header splits the whole bill, so these acts stay support under a
    // blank lead; promoting one of them would name an opener as headliner.
    expect(
      showEventDescription({
        ...holyWave,
        artists: [
          { name: '', set_type: 'headliner' },
          { name: 'Support A', set_type: 'opener' },
          { name: 'Support B' },
        ],
      })
    ).toBeNull()
    expect(
      showEventDescription({
        ...holyWave,
        artists: [{ name: '  ' }, { name: 'Holy Wave' }],
      })
    ).toBeNull()
  })

  it('is null when the bill names nobody', () => {
    expect(showEventDescription({ ...holyWave, artists: [{ name: ' ' }] })).toBeNull()
    expect(showEventDescription({ ...holyWave, artists: [] })).toBeNull()
    expect(showEventDescription({ ...holyWave, artists: null })).toBeNull()
  })

  it('drops the city when the venue has none', () => {
    expect(showEventDescription({ ...holyWave, city: '  ', doorsAt: DOORS })).toBe(
      'Holy Wave at Launchpad. Doors 7:00 PM.'
    )
  })

  it('drops the place, city included, when there is no venue', () => {
    expect(showEventDescription({ ...holyWave, venue: null })).toBe('Holy Wave.')
  })

  it('states a door time without a music time', () => {
    expect(showEventDescription({ ...holyWave, doorsAt: DOORS })).toBe(
      'Holy Wave at Launchpad, Albuquerque. Doors 7:00 PM.'
    )
  })

  it('states the music time beside a door time even when it is not the start time', () => {
    expect(
      showEventDescription({
        ...holyWave,
        doorsAt: DOORS,
        musicAt: '2026-11-22T03:30:00Z',
      })
    ).toBe('Holy Wave at Launchpad, Albuquerque. Doors 7:00 PM, music 8:30 PM.')
  })

  it('withholds a lone music time that is not the start time the page prints', () => {
    // Without a door time the page prints no music time, only the start time.
    expect(showEventDescription({ ...holyWave, musicAt: '2026-11-22T03:30:00Z' })).toBe(
      'Holy Wave at Launchpad, Albuquerque.'
    )
  })

  it('never states the start instant of a date-only show as a music time', () => {
    // A date-only show is stored at 20:00 venue-local with no announced times.
    expect(showEventDescription({ ...holyWave, doorsAt: null, musicAt: null })).toBe(
      'Holy Wave at Launchpad, Albuquerque.'
    )
  })

  it('states no time left on another night by a moved start', () => {
    // The start moved two weeks later; the stored door and music times did not.
    expect(
      showEventDescription({
        ...holyWave,
        doorsAt: '2026-11-08T02:00:00Z',
        musicAt: '2026-11-08T03:00:00Z',
      })
    ).toBe('Holy Wave at Launchpad, Albuquerque.')
  })

  it('judges the door and music times apart when only one is on the start night', () => {
    expect(
      showEventDescription({ ...holyWave, doorsAt: DOORS, musicAt: '2026-11-08T03:00:00Z' })
    ).toBe('Holy Wave at Launchpad, Albuquerque. Doors 7:00 PM.')
    expect(
      showEventDescription({ ...holyWave, doorsAt: '2026-11-08T02:00:00Z', musicAt: MUSIC })
    ).toBe('Holy Wave at Launchpad, Albuquerque. Music 8:00 PM.')
  })

  it('states no door time after the start', () => {
    expect(
      showEventDescription({ ...holyWave, doorsAt: '2026-11-22T05:00:00Z', musicAt: MUSIC })
    ).toBe('Holy Wave at Launchpad, Albuquerque. Music 8:00 PM.')
  })

  it('states no clock when the venue zone is a guess', () => {
    expect(
      showEventDescription({
        ...holyWave,
        state: 'England',
        timezone: null,
        doorsAt: DOORS,
        musicAt: MUSIC,
      })
    ).toBe('Holy Wave at Launchpad, Albuquerque.')
  })

  it('reads the clocks in the venue zone, not the state map', () => {
    // AZ alone would read 02:00Z as 7:00 PM; the venue's own zone says 8:00 PM.
    expect(
      showEventDescription({
        ...holyWave,
        state: 'AZ',
        timezone: 'America/Chicago',
        doorsAt: DOORS,
      })
    ).toBe('Holy Wave at Launchpad, Albuquerque. Doors 8:00 PM.')
  })

  it('skips an unreadable time rather than throwing', () => {
    expect(
      showEventDescription({ ...holyWave, doorsAt: 'not-a-date', musicAt: 'nope' })
    ).toBe('Holy Wave at Launchpad, Albuquerque.')
    expect(
      showEventDescription({ ...holyWave, eventDate: 'not-a-date', musicAt: MUSIC })
    ).toBe('Holy Wave at Launchpad, Albuquerque.')
  })

  it('separates the clock from AM/PM with a plain space', () => {
    const description = showEventDescription({ ...holyWave, doorsAt: DOORS, musicAt: MUSIC })
    expect(description).toContain('7:00 PM')
    expect(description).not.toMatch(/[\u00a0\u202f]/)
  })

  describe('states exactly the clocks the show page prints', () => {
    // The page's own rules: `doorsMusicFactSegment` (venue facts line) and
    // `startTimeFactSegment` (ticket line). Clocks are compared in the page's
    // compact register.
    const compact = (spoken: string) => spoken.replace(':00', '').replace(' ', '')
    const cases: Array<[string, Partial<ShowEventDescriptionInput>]> = [
      ['doors and music', { doorsAt: DOORS, musicAt: MUSIC }],
      ['doors and a music time after the start', { doorsAt: DOORS, musicAt: '2026-11-22T03:30:00Z' }],
      ['doors only', { doorsAt: DOORS }],
      ['music equal to the start', { musicAt: MUSIC }],
      ['music other than the start', { musicAt: '2026-11-22T03:30:00Z' }],
      ['no announced times', {}],
      ['half-hour doors', { doorsAt: '2026-11-22T02:30:00Z', musicAt: MUSIC }],
      ['midnight doors', { doorsAt: '2026-11-22T07:00:00Z', eventDate: '2026-11-22T07:00:00Z' }],
      ['a guessed zone', { doorsAt: DOORS, musicAt: MUSIC, state: 'England', timezone: null }],
      ['an invalid zone on a mapped state', { doorsAt: DOORS, musicAt: MUSIC, timezone: 'Not/AZone' }],
    ]

    it.each(cases)('%s', (_, over) => {
      const input = { ...holyWave, ...over }
      const description = showEventDescription(input) ?? ''
      const zone = { state: input.state, timezone: input.timezone }
      const pageTimes = doorsMusicFactSegment({ doorsAt: input.doorsAt, musicAt: input.musicAt, ...zone })
      const pageStart = startTimeFactSegment({ eventDate: input.eventDate, ...zone })

      const pageDoors = pageTimes?.match(/DOORS (\S+)/)?.[1] ?? null
      const pageMusic = pageTimes?.match(/MUSIC (\S+)/)?.[1] ?? null
      const doors = description.match(/Doors (\d+:\d+ [AP]M)/)?.[1]
      const music = description.match(/[Mm]usic (\d+:\d+ [AP]M)/)?.[1]

      expect(doors ? compact(doors) : null).toBe(pageDoors)
      if (pageMusic) expect(music && compact(music)).toBe(pageMusic)
      if (music) {
        // A music time the page prints: beside doors, or as the lone start.
        expect([pageMusic, pageDoors ? null : pageStart]).toContain(compact(music))
      }
    })
  })

  it('ends the sentence once when a name already ends with a full stop', () => {
    expect(showEventDescription({ ...holyWave, venue: 'Launchpad', city: 'St.' })).toBe(
      'Holy Wave at Launchpad, St.'
    )
  })
})

/** Code points, as the budgets count them. */
function charLength(text: string): number {
  return Array.from(text).length
}

/** A name of `n` code points, for budget tests. */
function nameOf(n: number, letter = 'n'): string {
  return letter.repeat(n)
}

describe('statedUpcomingCount', () => {
  const live = { is_cancelled: false }
  const called = { is_cancelled: true }

  it('states the payload total when a listed show is not cancelled', () => {
    expect(statedUpcomingCount({ total: 3, shows: [called, live] })).toBe(3)
  })

  it('states nothing when every listed show is cancelled', () => {
    expect(statedUpcomingCount({ total: 2, shows: [called, called] })).toBeNull()
  })

  it('states nothing for no rows, a zero total or no payload', () => {
    expect(statedUpcomingCount({ total: 4, shows: [] })).toBeNull()
    expect(statedUpcomingCount({ total: 0, shows: [live] })).toBeNull()
    expect(statedUpcomingCount(null)).toBeNull()
    expect(statedUpcomingCount(undefined)).toBeNull()
  })
})

describe('withoutEndedShows', () => {
  // 10:00 AM on November 20, 2026 in Phoenix.
  const NOW = new Date('2026-11-20T17:00:00Z')
  const PHOENIX = () => ({ state: 'AZ', timezone: 'America/Phoenix' })
  const row = (event_date: string) => ({ event_date, is_cancelled: false })

  it("drops last night's show and lowers the total by the same count", () => {
    const lastNight = row('2026-11-20T03:00:00Z') // 8:00 PM Nov 19 in Phoenix
    const tonight = row('2026-11-21T03:00:00Z') // 8:00 PM Nov 20 in Phoenix
    expect(withoutEndedShows({ total: 5, shows: [lastNight, tonight] }, PHOENIX, NOW)).toEqual({
      total: 4,
      shows: [tonight],
    })
  })

  it("keeps a show that started earlier today until the venue's midnight", () => {
    const matinee = row('2026-11-20T15:00:00Z') // 8:00 AM Nov 20 in Phoenix
    expect(withoutEndedShows({ total: 1, shows: [matinee] }, PHOENIX, NOW).shows).toEqual([matinee])
  })

  it("reads each row's day on its own venue's calendar", () => {
    // 3:00 AM Nov 20 in Phoenix and 7:00 PM Nov 20 in Tokyo. The show is
    // 11:00 PM Nov 19 in Phoenix (ended) and 3:00 PM Nov 20 in Tokyo (today).
    const now = new Date('2026-11-20T10:00:00Z')
    const show = row('2026-11-20T06:00:00Z')
    expect(withoutEndedShows({ total: 1, shows: [show] }, PHOENIX, now).total).toBe(0)
    expect(
      withoutEndedShows({ total: 1, shows: [show] }, () => ({ timezone: 'Asia/Tokyo' }), now).total
    ).toBe(1)
  })

  it('drops an unreadable date and never goes below zero', () => {
    expect(withoutEndedShows({ total: 0, shows: [row('garbage')] }, PHOENIX, NOW)).toEqual({
      total: 0,
      shows: [],
    })
  })
})

describe('artistNextShowFrom', () => {
  const row = (overrides: Record<string, unknown> = {}) => ({
    event_date: '2026-12-03T02:00:00Z',
    is_cancelled: false,
    venue: {
      name: 'Lincoln Hall',
      city: 'Chicago',
      state: 'IL',
      timezone: 'America/Chicago',
    },
    ...overrides,
  })

  it('names the first show that is on, on its venue calendar', () => {
    expect(
      artistNextShowFrom([
        row({ is_cancelled: true, venue: { name: 'Called Off', city: 'X' } }),
        row({ event_date: 'garbage', venue: { name: 'Undated', city: 'X' } }),
        row(),
      ])
    ).toEqual({
      venue: 'Lincoln Hall',
      venueCity: 'Chicago',
      timing: {
        eventDate: '2026-12-03T02:00:00Z',
        state: 'IL',
        timezone: 'America/Chicago',
      },
    })
  })

  it('names no later show when the next one has no venue', () => {
    expect(artistNextShowFrom([row({ venue: null }), row()])).toBeNull()
    expect(artistNextShowFrom([row({ venue: { name: '  ' } }), row()])).toBeNull()
  })

  it('is null for an empty or all-cancelled list', () => {
    expect(artistNextShowFrom([])).toBeNull()
    expect(artistNextShowFrom([row({ is_cancelled: true })])).toBeNull()
  })
})

describe('artistSnippet', () => {
  const NEXT = {
    venue: 'Lincoln Hall',
    venueCity: 'Chicago',
    timing: {
      eventDate: '2026-12-03T02:00:00Z',
      state: 'IL',
      timezone: 'America/Chicago',
    },
  }
  const POKEY = {
    name: 'Pokey LaFarge',
    city: 'Bloomington',
    state: 'MN',
    nextShow: NEXT,
    upcomingCount: 2,
    releaseCount: 9,
    similarArtistCount: 14,
  }

  it('renders every sentence for an artist with shows coming', () => {
    const { title, description } = artistSnippet(POKEY)
    expect(title).toBe('Pokey LaFarge · Bloomington, MN')
    expect(description).toBe(
      'Pokey LaFarge from Bloomington, MN. Next show: Lincoln Hall, Chicago, Dec 2. Upcoming shows: 2. Releases: 9. Similar artists: 14.'
    )
  })

  it('drops `from` when there is no location', () => {
    const { title, description } = artistSnippet({
      name: 'Ben Watt and Tracey Thorn',
      nextShow: { ...NEXT, venue: 'Moth Club', venueCity: 'London' },
      upcomingCount: 3,
      releaseCount: 0,
      similarArtistCount: 0,
    })
    expect(title).toBe('Ben Watt and Tracey Thorn')
    expect(description).toBe(
      'Ben Watt and Tracey Thorn. Next show: Moth Club, London, Dec 2. Upcoming shows: 3.'
    )
  })

  it('says nothing about shows for an artist with none coming', () => {
    expect(
      artistSnippet({
        name: 'The Body',
        city: 'Little Rock',
        state: 'AR',
        nextShow: null,
        upcomingCount: null,
        releaseCount: 34,
        similarArtistCount: 4,
      }).description
    ).toBe('The Body from Little Rock, AR. Releases: 34. Similar artists: 4.')
  })

  it('leaves the lead alone when nothing is on record', () => {
    expect(artistSnippet({ name: 'Uniform', city: 'New York', state: 'NY' }).description).toBe(
      'Uniform from New York, NY.'
    )
    expect(
      artistSnippet({
        name: 'Uniform',
        upcomingCount: 0,
        releaseCount: 0,
        similarArtistCount: 0,
      }).description
    ).toBe('Uniform.')
  })

  it('names the venue alone when it has no city', () => {
    expect(
      artistSnippet({ name: 'Band', nextShow: { ...NEXT, venueCity: null }, upcomingCount: 1 })
        .description
    ).toBe('Band. Next show: Lincoln Hall, Dec 2. Upcoming shows: 1.')
  })

  it('omits the next show when its date is unreadable', () => {
    expect(
      artistSnippet({
        name: 'Band',
        nextShow: { ...NEXT, timing: { eventDate: 'garbage' } },
        upcomingCount: 1,
      }).description
    ).toBe('Band. Upcoming shows: 1.')
  })

  it('groups counts in en-US form', () => {
    expect(
      artistSnippet({ name: 'Band', releaseCount: 1234, similarArtistCount: 1 }).description
    ).toBe('Band. Releases: 1,234. Similar artists: 1.')
  })

  it('ignores counts that are not positive whole numbers', () => {
    expect(
      artistSnippet({ name: 'Band', releaseCount: -2, similarArtistCount: 1.5, upcomingCount: Number.NaN })
        .description
    ).toBe('Band.')
  })

  it('never states shows tracked', () => {
    expect(artistSnippet(POKEY).description).not.toMatch(/tracked/i)
  })

  describe('drops sentences from the end to fit the budget', () => {
    const render = (n: number) =>
      artistSnippet({ ...POKEY, name: nameOf(n), city: null, state: null }).description
    // Everything but the name when every sentence is kept.
    const fixed = charLength(render(1)) - 1
    const similar = charLength(' Similar artists: 14.')
    const releases = charLength(' Releases: 9.')
    const upcoming = charLength(' Upcoming shows: 2.')
    const fits = DESCRIPTION_BUDGET - fixed

    it('keeps every sentence at exactly the budget', () => {
      expect(charLength(render(fits))).toBe(DESCRIPTION_BUDGET)
      expect(render(fits).endsWith('Similar artists: 14.')).toBe(true)
    })

    it('drops Similar artists first', () => {
      expect(render(fits + 1).endsWith('Releases: 9.')).toBe(true)
    })

    it('then Releases', () => {
      expect(render(fits + similar + 1).endsWith('Upcoming shows: 2.')).toBe(true)
    })

    it('then Upcoming shows', () => {
      expect(
        render(fits + similar + releases + 1).endsWith('Next show: Lincoln Hall, Chicago, Dec 2.')
      ).toBe(true)
    })

    it('then Next show, leaving the lead', () => {
      const n = fits + similar + releases + upcoming + 1
      expect(render(n)).toBe(`${nameOf(n)}.`)
    })

    it('cuts a lead that is over budget by itself', () => {
      const text = render(200)
      expect(charLength(text)).toBe(DESCRIPTION_BUDGET)
      expect(text.endsWith('...')).toBe(true)
    })
  })

  it('names the city alone in the title when there is no state', () => {
    expect(artistSnippet({ name: 'Band', city: 'Melbourne', state: null }).title).toBe(
      'Band · Melbourne'
    )
  })

  it('drops the location from the title when it does not fit', () => {
    const name = 'x'.repeat(35)
    const { title, description } = artistSnippet({ name, city: 'San Francisco', state: 'CA' })
    expect(title).toBe(name)
    expect(description).toBe(`${name} from San Francisco, CA.`)
  })
})

describe('venueSnippet', () => {
  const THALIA = {
    name: 'Thalia Hall',
    city: 'Chicago',
    state: 'IL',
    address: '1807 S Allport St',
    currentYear: 2026,
  }
  const SQUIRREL = {
    headliner: 'Squirrel Flower',
    timing: {
      eventDate: '2026-11-21T02:00:00Z',
      state: 'IL',
      timezone: 'America/Chicago',
    },
  }
  const PAST = { years: [{ year: 2026, count: 50 }, { year: 2025, count: 6 }] }

  describe('a room with shows coming', () => {
    it('renders every sentence', () => {
      const { title, description } = venueSnippet({
        ...THALIA,
        upcomingCount: 7,
        nextShow: SQUIRREL,
        pastRecord: PAST,
      })
      expect(title).toBe('Thalia Hall shows · Chicago, IL')
      expect(description).toBe(
        '7 upcoming shows at Thalia Hall in Chicago, IL. Next: Squirrel Flower, Nov 20. 56 past shows on record since 2025. Address: 1807 S Allport St.'
      )
    })

    it('uses the singular for one show and one past show', () => {
      expect(
        venueSnippet({
          ...THALIA,
          address: null,
          upcomingCount: 1,
          nextShow: SQUIRREL,
          pastRecord: { years: [{ year: 2026, count: 1 }] },
        }).description
      ).toBe(
        '1 upcoming show at Thalia Hall in Chicago, IL. Next: Squirrel Flower, Nov 20. 1 past show on record.'
      )
    })

    it('omits Next when there is no next show or its date is unreadable', () => {
      expect(
        venueSnippet({ ...THALIA, address: null, upcomingCount: 2, nextShow: null }).description
      ).toBe('2 upcoming shows at Thalia Hall in Chicago, IL.')
      expect(
        venueSnippet({
          ...THALIA,
          address: null,
          upcomingCount: 2,
          nextShow: { headliner: 'Band', timing: { eventDate: 'garbage' } },
        }).description
      ).toBe('2 upcoming shows at Thalia Hall in Chicago, IL.')
    })

    it('marks the Next date when the venue zone is a guess', () => {
      expect(
        venueSnippet({
          name: 'Room',
          city: 'Toronto',
          state: 'ON',
          currentYear: 2026,
          upcomingCount: 1,
          nextShow: { headliner: 'Band', timing: { eventDate: '2026-09-12T03:00:00Z', state: 'ON' } },
        }).description
      ).toBe('1 upcoming show at Room in Toronto, ON. Next: Band, ~Sep 11.')
    })

    it('groups counts in en-US form', () => {
      expect(
        venueSnippet({
          ...THALIA,
          address: null,
          upcomingCount: 1200,
          pastRecord: { years: [{ year: 2026, count: 2345 }] },
        }).description
      ).toBe('1,200 upcoming shows at Thalia Hall in Chicago, IL. 2,345 past shows on record.')
    })

    describe('drops sentences from the end to fit the budget', () => {
      const input = (n: number) => ({
        ...THALIA,
        name: nameOf(n, 'v'),
        upcomingCount: 7,
        nextShow: SQUIRREL,
        pastRecord: PAST,
      })
      const full = (n: number) => venueSnippet(input(n)).description
      // Fixed text around the name when every sentence is kept (measured on a
      // one-letter name: an empty one would collapse the space around it).
      const fixed = charLength(full(1)) - 1

      it('keeps every sentence at exactly the budget', () => {
        const text = full(DESCRIPTION_BUDGET - fixed)
        expect(charLength(text)).toBe(DESCRIPTION_BUDGET)
        expect(text.endsWith('Address: 1807 S Allport St.')).toBe(true)
      })

      it('drops Address first', () => {
        expect(full(DESCRIPTION_BUDGET - fixed + 1).endsWith('on record since 2025.')).toBe(true)
      })

      const address = charLength(' Address: 1807 S Allport St.')
      const past = charLength(' 56 past shows on record since 2025.')

      it('then the past shows', () => {
        expect(
          full(DESCRIPTION_BUDGET - fixed + address + 1).endsWith('Next: Squirrel Flower, Nov 20.')
        ).toBe(true)
      })

      it('then Next, leaving the lead', () => {
        const n = DESCRIPTION_BUDGET - fixed + address + past + 1
        expect(full(n)).toBe(`7 upcoming shows at ${nameOf(n, 'v')} in Chicago, IL.`)
      })
    })
  })

  describe('the since rule', () => {
    const past = (currentYear: number, years: Array<{ year: number; count: number }>) =>
      venueSnippet({ ...THALIA, address: null, currentYear, pastRecord: { years } }).description

    it('names the first year when it is before the current year', () => {
      expect(past(2027, [{ year: 2026, count: 3 }])).toBe(
        'Thalia Hall in Chicago, IL: 3 past shows on record since 2026.'
      )
    })

    it('names no year when the first year is the current year', () => {
      expect(past(2026, [{ year: 2026, count: 3 }])).toBe(
        'Thalia Hall in Chicago, IL: 3 past shows on record.'
      )
    })

    it('names no year when the first year is after the current year', () => {
      expect(past(2025, [{ year: 2026, count: 3 }])).toBe(
        'Thalia Hall in Chicago, IL: 3 past shows on record.'
      )
    })

    it('takes the earliest year whatever the order', () => {
      expect(
        past(2026, [{ year: 2019, count: 1 }, { year: 2024, count: 1 }, { year: 2021, count: 1 }])
      ).toBe('Thalia Hall in Chicago, IL: 3 past shows on record since 2019.')
    })
  })

  describe('a room with only past shows', () => {
    it('leads with the past record, then the address', () => {
      expect(
        venueSnippet({
          name: 'Crescent Ballroom',
          city: 'Phoenix',
          state: 'AZ',
          address: '308 N 2nd Ave',
          currentYear: 2027,
          pastRecord: { years: [{ year: 2026, count: 12 }] },
        }).description
      ).toBe('Crescent Ballroom in Phoenix, AZ: 12 past shows on record since 2026. Address: 308 N 2nd Ave.')
    })

    it('treats a room whose upcoming shows are all cancelled as having none to state', () => {
      const upcomingCount = statedUpcomingCount({
        total: 2,
        shows: [{ is_cancelled: true }, { is_cancelled: true }],
      })
      expect(
        venueSnippet({
          ...THALIA,
          address: null,
          upcomingCount,
          nextShow: null,
          pastRecord: { years: [{ year: 2026, count: 1 }] },
        }).description
      ).toBe('Thalia Hall in Chicago, IL: 1 past show on record.')
    })

    it('drops the address to fit the budget', () => {
      const render = (n: number) =>
        venueSnippet({
          name: nameOf(n, 'v'),
          city: 'Phoenix',
          state: 'AZ',
          address: '308 N 2nd Ave',
          currentYear: 2026,
          pastRecord: { years: [{ year: 2026, count: 1 }] },
        }).description
      const fits = DESCRIPTION_BUDGET - (charLength(render(1)) - 1)
      expect(charLength(render(fits))).toBe(DESCRIPTION_BUDGET)
      expect(render(fits).endsWith('Address: 308 N 2nd Ave.')).toBe(true)
      expect(render(fits + 1)).toBe(`${nameOf(fits + 1, 'v')} in Phoenix, AZ: 1 past show on record.`)
    })
  })

  describe('a room with nothing on record', () => {
    it('says it is a venue, with its address when there is one', () => {
      expect(
        venueSnippet({ name: 'Douglas Park', city: 'Chicago', state: 'IL', currentYear: 2026 })
          .description
      ).toBe('Douglas Park is a venue in Chicago, IL.')
      expect(
        venueSnippet({ ...THALIA, pastRecord: { years: [] } }).description
      ).toBe('Thalia Hall is a venue in Chicago, IL. Address: 1807 S Allport St.')
    })

    it('drops the address to fit the budget', () => {
      const render = (n: number) =>
        venueSnippet({ ...THALIA, name: nameOf(n, 'v'), pastRecord: null }).description
      const fits = DESCRIPTION_BUDGET - (charLength(render(1)) - 1)
      expect(charLength(render(fits))).toBe(DESCRIPTION_BUDGET)
      expect(render(fits).endsWith('Address: 1807 S Allport St.')).toBe(true)
      expect(render(fits + 1)).toBe(`${nameOf(fits + 1, 'v')} is a venue in Chicago, IL.`)
    })

    it('drops the place when the room has none', () => {
      const { title, description } = venueSnippet({
        name: 'Lost Bag',
        city: '',
        state: '',
        currentYear: 2026,
      })
      expect(title).toBe('Lost Bag shows')
      expect(description).toBe('Lost Bag is a venue.')
    })

    it('ignores zero and malformed histogram rows', () => {
      expect(
        venueSnippet({
          ...THALIA,
          address: null,
          pastRecord: { years: [{ year: 2020, count: 0 }, { year: Number.NaN, count: 4 }] },
        }).description
      ).toBe('Thalia Hall is a venue in Chicago, IL.')
    })
  })

  describe('the title', () => {
    it('drops the place first, then `shows`; the name is never cut', () => {
      const thirty = 'v'.repeat(30)
      expect(venueSnippet({ name: thirty, city: 'Providence', state: 'RI', currentYear: 2026 }).title).toBe(
        `${thirty} shows`
      )
      const forty = 'v'.repeat(40)
      expect(venueSnippet({ name: forty, city: 'Providence', state: 'RI', currentYear: 2026 }).title).toBe(forty)
      const seventy = 'v'.repeat(70)
      expect(venueSnippet({ name: seventy, city: 'Providence', state: 'RI', currentYear: 2026 }).title).toBe(seventy)
    })

    it('fits the budget with the place kept', () => {
      const { title } = venueSnippet({ ...THALIA })
      expect(titleLength(title)).toBeLessThanOrEqual(TITLE_BUDGET)
      expect(titleLength(title)).toBe(48)
    })
  })
})

describe('releaseArtistPhrase', () => {
  it('names one artist', () => {
    expect(releaseArtistPhrase(['Tim Atlas'])).toBe('Tim Atlas')
  })

  it('joins two artists with "and"', () => {
    expect(releaseArtistPhrase(['Band A', 'Band B'])).toBe('Band A and Band B')
  })

  it('names the first of three or more followed by "and others"', () => {
    expect(releaseArtistPhrase(['Band A', 'Band B', 'Band C'])).toBe(
      'Band A and others'
    )
    expect(releaseArtistPhrase(['A', 'B', 'C', 'D', 'E'])).toBe('A and others')
  })

  it('skips blank names and trims the rest', () => {
    expect(releaseArtistPhrase([' ', '  Band A ', ''])).toBe('Band A')
  })

  it('is null for no artists', () => {
    expect(releaseArtistPhrase([])).toBeNull()
    expect(releaseArtistPhrase(null)).toBeNull()
    expect(releaseArtistPhrase(undefined)).toBeNull()
  })
})

describe('releaseSnippet', () => {
  const EYES_SHUT = {
    title: 'Eyes Shut',
    artists: ['Tim Atlas'],
    typeLabel: 'EP',
    year: 2019,
    label: 'Nettwerk',
    listenServices: ['Bandcamp', 'Spotify'],
  }

  it('renders the strings the owner approved for one artist', () => {
    const { title, description } = releaseSnippet(EYES_SHUT)
    expect(title).toBe('Eyes Shut by Tim Atlas (2019)')
    expect(title + SITE_TITLE_SUFFIX).toBe(
      'Eyes Shut by Tim Atlas (2019) | Psychic Homily'
    )
    expect(description).toBe(
      'Eyes Shut, EP by Tim Atlas (2019). Label: Nettwerk. Listen: Bandcamp, Spotify.'
    )
  })

  it('joins two artists with "and" in both strings', () => {
    const { title, description } = releaseSnippet({
      title: 'Split',
      artists: ['Band A', 'Band B'],
      typeLabel: 'LP',
      year: 2021,
    })
    expect(title).toBe('Split by Band A and Band B (2021)')
    expect(description).toBe('Split, LP by Band A and Band B (2021).')
  })

  it('names the first of more than two artists with "and others"', () => {
    const { title, description } = releaseSnippet({
      title: 'Comp',
      artists: ['Band A', 'Band B', 'Band C'],
      typeLabel: 'Compilation',
      year: 2021,
    })
    expect(title).toBe('Comp by Band A and others (2021)')
    expect(description).toBe('Comp, Compilation by Band A and others (2021).')
  })

  it('drops the year clause from both strings when there is no year', () => {
    const { title, description } = releaseSnippet({ ...EYES_SHUT, year: null })
    expect(title).toBe('Eyes Shut by Tim Atlas')
    expect(description).toBe(
      'Eyes Shut, EP by Tim Atlas. Label: Nettwerk. Listen: Bandcamp, Spotify.'
    )
  })

  it('drops the year first when the title is over budget', () => {
    // With the year: 36 + 7 + 17 = 60 fits; one more character does not.
    const fits = releaseSnippet({ ...EYES_SHUT, title: 'x'.repeat(23) })
    expect(fits.title).toBe(`${'x'.repeat(23)} by Tim Atlas (2019)`)
    expect(titleLength(fits.title)).toBe(TITLE_BUDGET)
    const over = releaseSnippet({ ...EYES_SHUT, title: 'x'.repeat(24) })
    expect(over.title).toBe(`${'x'.repeat(24)} by Tim Atlas`)
    // The description keeps the year: only the title has the tighter budget.
    expect(over.description).toContain('by Tim Atlas (2019).')
  })

  it('keeps the title and artist whole, without an ellipsis, when they alone are over budget', () => {
    const longTitle = 'All The Young Droids: Junkshop Synth Pop 1978-1985'
    const { title } = releaseSnippet({
      title: longTitle,
      artists: ['Gerry and the Holograms', 'Peta Lily & Michael Process'],
      year: 2019,
    })
    expect(title).toBe(
      `${longTitle} by Gerry and the Holograms and Peta Lily & Michael Process`
    )
    expect(titleLength(title)).toBeGreaterThan(TITLE_BUDGET)
    expect(title).not.toContain('...')
  })

  it('drops the "by" clause from both strings when there is no artist', () => {
    const { title, description } = releaseSnippet({ ...EYES_SHUT, artists: [] })
    expect(title).toBe('Eyes Shut (2019)')
    expect(description).toBe(
      'Eyes Shut, EP (2019). Label: Nettwerk. Listen: Bandcamp, Spotify.'
    )
    expect(releaseSnippet({ ...EYES_SHUT, artists: null }).title).toBe(
      'Eyes Shut (2019)'
    )
  })

  it('drops the type clause when there is no type', () => {
    for (const typeLabel of [null, '', '  ']) {
      const { title, description } = releaseSnippet({ ...EYES_SHUT, typeLabel })
      expect(title).toBe('Eyes Shut by Tim Atlas (2019)')
      expect(description).toBe(
        'Eyes Shut by Tim Atlas (2019). Label: Nettwerk. Listen: Bandcamp, Spotify.'
      )
    }
  })

  it('drops the Label clause when there is no label', () => {
    for (const label of [null, undefined, ' ']) {
      expect(releaseSnippet({ ...EYES_SHUT, label }).description).toBe(
        'Eyes Shut, EP by Tim Atlas (2019). Listen: Bandcamp, Spotify.'
      )
    }
  })

  it('drops the Listen clause when there is no service to name', () => {
    for (const listenServices of [[], null, [' ']]) {
      expect(releaseSnippet({ ...EYES_SHUT, listenServices }).description).toBe(
        'Eyes Shut, EP by Tim Atlas (2019). Label: Nettwerk.'
      )
    }
  })

  it('is the bare title when nothing else is known', () => {
    const { title, description } = releaseSnippet({ title: 'Untitled' })
    expect(title).toBe('Untitled')
    expect(description).toBe('Untitled.')
  })

  it('does not double the full stop after a value that ends in one', () => {
    const { description } = releaseSnippet({
      title: 'The Derivative Sounds of...',
      label: 'Records Inc.',
    })
    expect(description).toBe('The Derivative Sounds of... Label: Records Inc.')
  })

  // `{title}, EP by Tim Atlas (2019).` is the title plus 25 characters,
  // ` Label: Nettwerk.` is 17 and ` Listen: Bandcamp, Spotify.` is 27.
  it('drops the Listen sentence whole when it is the one that does not fit', () => {
    // 100 + 25 + 17 = 142 fits; adding Listen makes 169.
    const title = 't'.repeat(100)
    const { description } = releaseSnippet({ ...EYES_SHUT, title })
    expect(description).toBe(`${title}, EP by Tim Atlas (2019). Label: Nettwerk.`)
  })

  it('keeps both sentences when they land exactly on the budget', () => {
    const title = 't'.repeat(DESCRIPTION_BUDGET - 25 - 17 - 27)
    const { description } = releaseSnippet({ ...EYES_SHUT, title })
    expect(description).toHaveLength(DESCRIPTION_BUDGET)
    expect(description.endsWith('Listen: Bandcamp, Spotify.')).toBe(true)
  })

  it('drops the Label sentence too when only the first sentence fits', () => {
    // 120 + 25 = 145 fits; adding Label makes 162.
    const title = 't'.repeat(120)
    const { description } = releaseSnippet({ ...EYES_SHUT, title })
    expect(description).toBe(`${title}, EP by Tim Atlas (2019).`)
  })

  it('cuts the first sentence with an ellipsis when it alone is over budget', () => {
    const { description } = releaseSnippet({ ...EYES_SHUT, title: 't'.repeat(200) })
    expect(description).toBe('t'.repeat(DESCRIPTION_BUDGET - 3) + '...')
  })

  it('has no em dash in any output', () => {
    const { title, description } = releaseSnippet(EYES_SHUT)
    expect(title + description).not.toContain('\u2014')
  })
})

describe('venueNextShowFrom', () => {
  const VENUE = { state: 'RI', timezone: 'America/New_York' }
  const row = (overrides: Record<string, unknown> = {}) => ({
    event_date: '2026-10-04T00:00:00Z',
    is_cancelled: false,
    state: null,
    title: 'Show Title',
    artists: [
      { name: 'Opener', is_headliner: false },
      { name: 'Top Billing', is_headliner: true },
    ],
    ...overrides,
  })

  it('names the headliner of the first row, on the venue calendar', () => {
    expect(venueNextShowFrom([row()], VENUE)).toEqual({
      headliner: 'Top Billing',
      timing: {
        eventDate: '2026-10-04T00:00:00Z',
        state: 'RI',
        timezone: 'America/New_York',
      },
    })
  })

  it('skips cancelled and undated rows', () => {
    const next = venueNextShowFrom(
      [
        row({ is_cancelled: true, artists: [{ name: 'Called Off', is_headliner: true }] }),
        row({ event_date: 'garbage', artists: [{ name: 'Undated', is_headliner: true }] }),
        row({ artists: [{ name: 'Still On', is_headliner: true }] }),
      ],
      VENUE
    )
    expect(next?.headliner).toBe('Still On')
  })

  it('falls back to the first artist, then the title', () => {
    expect(
      venueNextShowFrom([row({ artists: [{ name: 'First' }] })], VENUE)?.headliner
    ).toBe('First')
    expect(
      venueNextShowFrom([row({ artists: [] })], VENUE)?.headliner
    ).toBe('Show Title')
  })

  it('lets the row state outrank the venue state', () => {
    expect(
      venueNextShowFrom([row({ state: 'MA' })], VENUE)?.timing.state
    ).toBe('MA')
  })

  it('is null for an empty list or rows with nothing to name', () => {
    expect(venueNextShowFrom([], VENUE)).toBeNull()
    expect(
      venueNextShowFrom([row({ artists: [], title: '' })], VENUE)
    ).toBeNull()
  })
})

describe('labelDescription', () => {
  const DAIS = {
    name: 'Dais Records',
    city: 'Brooklyn',
    state: 'NY',
    foundedYear: 2007,
    status: 'active',
    artistCount: 89,
    releaseCount: 921,
  }

  it('renders both sentences', () => {
    expect(labelDescription(DAIS)).toBe(
      'Dais Records is a label from Brooklyn, NY, founded in 2007. 89 artists and 921 releases on record.'
    )
  })

  it('drops the place and the founding year when they are unknown', () => {
    expect(
      labelDescription({ name: '15 Love', artistCount: 1, releaseCount: 1 })
    ).toBe('15 Love is a label. 1 artist and 1 release on record.')
    expect(labelDescription({ ...DAIS, foundedYear: null })).toBe(
      'Dais Records is a label from Brooklyn, NY. 89 artists and 921 releases on record.'
    )
    expect(labelDescription({ ...DAIS, city: 'Sheffield', state: null, foundedYear: 0 })).toBe(
      'Dais Records is a label from Sheffield. 89 artists and 921 releases on record.'
    )
  })

  it('drops the half whose count is zero, and the sentence when both are', () => {
    expect(labelDescription({ ...DAIS, releaseCount: 0 })).toBe(
      'Dais Records is a label from Brooklyn, NY, founded in 2007. 89 artists on record.'
    )
    expect(labelDescription({ ...DAIS, artistCount: 0, releaseCount: 1 })).toBe(
      'Dais Records is a label from Brooklyn, NY, founded in 2007. 1 release on record.'
    )
    expect(labelDescription({ ...DAIS, artistCount: 0, releaseCount: 0 })).toBe(
      'Dais Records is a label from Brooklyn, NY, founded in 2007.'
    )
  })

  it('names a label that is not active', () => {
    expect(labelDescription({ ...DAIS, status: 'defunct' })).toMatch(
      /^Dais Records is a defunct label from Brooklyn, NY/
    )
    expect(labelDescription({ ...DAIS, status: 'inactive' })).toMatch(
      /^Dais Records is an inactive label from Brooklyn, NY/
    )
    expect(labelDescription({ ...DAIS, status: ' Defunct ' })).toMatch(/is a defunct label/)
  })

  it('reads any other status as a label', () => {
    for (const status of ['active', 'constructor', '__proto__', '', null]) {
      expect(labelDescription({ ...DAIS, status })).toMatch(/^Dais Records is a label from/)
    }
  })

  it('groups counts in en-US form', () => {
    expect(labelDescription({ name: 'L', artistCount: 1234, releaseCount: 56789 })).toBe(
      'L is a label. 1,234 artists and 56,789 releases on record.'
    )
  })

  it('drops the counts sentence to fit the budget, then cuts the lead', () => {
    const counts = charLength(' 89 artists and 921 releases on record.')
    const lead = (n: number) => `${nameOf(n, 'l')} is a label from Brooklyn, NY, founded in 2007.`
    const leadFixed = charLength(lead(0))
    const fits = DESCRIPTION_BUDGET - leadFixed - counts
    expect(labelDescription({ ...DAIS, name: nameOf(fits, 'l') }).endsWith('921 releases on record.')).toBe(true)
    expect(labelDescription({ ...DAIS, name: nameOf(fits + 1, 'l') })).toBe(lead(fits + 1))
    const cut = labelDescription({ ...DAIS, name: nameOf(200, 'l') })
    expect(charLength(cut)).toBe(DESCRIPTION_BUDGET)
    expect(cut.endsWith('...')).toBe(true)
  })
})

describe('festivalDateRange', () => {
  it('writes one day, one month, two months and two years', () => {
    expect(festivalDateRange('2026-05-30', '2026-05-30')).toBe('May 30, 2026')
    expect(festivalDateRange('2026-09-18', '2026-09-20')).toBe('Sep 18 to 20, 2026')
    expect(festivalDateRange('2026-09-30', '2026-10-02')).toBe('Sep 30 to Oct 2, 2026')
    expect(festivalDateRange('2026-12-30', '2027-01-02')).toBe('Dec 30, 2026 to Jan 2, 2027')
  })

  it('reads the calendar date, not an instant', () => {
    // Midnight UTC would be the previous evening west of Greenwich.
    expect(festivalDateRange('2026-01-01', '2026-01-01')).toBe('Jan 1, 2026')
  })

  it('gives the start alone when the end is unreadable, and null without a start', () => {
    expect(festivalDateRange('2026-09-18', 'soon')).toBe('Sep 18, 2026')
    expect(festivalDateRange('2026-09-18', null)).toBe('Sep 18, 2026')
    expect(festivalDateRange('2026-02-30', '2026-03-01')).toBeNull()
    expect(festivalDateRange('', '2026-03-01')).toBeNull()
    expect(festivalDateRange(null, null)).toBeNull()
  })
})

describe('festivalDescription', () => {
  const RIOT = {
    name: 'Riot Fest 2026',
    startDate: '2026-09-18',
    endDate: '2026-09-20',
    locationName: 'Douglas Park',
    city: 'Chicago',
    state: 'IL',
    status: 'announced',
    artistCount: 102,
    venueCount: 3,
  }

  it('renders every sentence', () => {
    expect(festivalDescription(RIOT)).toBe(
      'Riot Fest 2026: Sep 18 to 20, 2026, Douglas Park, Chicago, IL. Lineup: 102 artists. Venues: 3.'
    )
  })

  it('states the venue count only above one', () => {
    expect(festivalDescription({ ...RIOT, venueCount: 1 })).toBe(
      'Riot Fest 2026: Sep 18 to 20, 2026, Douglas Park, Chicago, IL. Lineup: 102 artists.'
    )
    expect(festivalDescription({ ...RIOT, venueCount: 0 })).not.toContain('Venues')
    expect(festivalDescription({ ...RIOT, venueCount: 2 })).toContain('Venues: 2.')
  })

  it('states the lineup only when it is not empty, in the singular for one', () => {
    expect(festivalDescription({ ...RIOT, artistCount: 1, venueCount: 0 })).toBe(
      'Riot Fest 2026: Sep 18 to 20, 2026, Douglas Park, Chicago, IL. Lineup: 1 artist.'
    )
    expect(festivalDescription({ ...RIOT, artistCount: 0, venueCount: 0 })).toBe(
      'Riot Fest 2026: Sep 18 to 20, 2026, Douglas Park, Chicago, IL.'
    )
  })

  it('marks a cancelled festival', () => {
    expect(festivalDescription({ ...RIOT, status: 'cancelled', venueCount: 0 })).toBe(
      'Riot Fest 2026 (cancelled): Sep 18 to 20, 2026, Douglas Park, Chicago, IL. Lineup: 102 artists.'
    )
  })

  it('drops each missing part of the lead', () => {
    expect(
      festivalDescription({
        name: 'Desert Fox Fest 2026',
        startDate: '2026-05-30',
        endDate: '2026-05-30',
        city: 'Piacenza',
        state: null,
        artistCount: 8,
      })
    ).toBe('Desert Fox Fest 2026: May 30, 2026, Piacenza. Lineup: 8 artists.')
    expect(festivalDescription({ name: 'Fest', startDate: 'tbd' })).toBe('Fest.')
  })

  it('drops Venues first, then Lineup, to fit the budget', () => {
    const lineup = charLength(' Lineup: 102 artists.')
    const venues = charLength(' Venues: 3.')
    const leadFixed = charLength(festivalDescription({ ...RIOT, name: 'x', artistCount: 0, venueCount: 0 })) - 1
    const all = DESCRIPTION_BUDGET - leadFixed - lineup - venues
    const named = (n: number) => festivalDescription({ ...RIOT, name: nameOf(n, 'f') })
    expect(named(all).endsWith('Venues: 3.')).toBe(true)
    expect(named(all + 1).endsWith('Lineup: 102 artists.')).toBe(true)
    expect(named(all + venues + 1).endsWith('Chicago, IL.')).toBe(true)
  })
})
