import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  DESCRIPTION_BUDGET,
  SITE_TITLE_SUFFIX,
  TITLE_BUDGET,
  artistSnippet,
  fitDescription,
  fitTitle,
  showSnippet,
  snippetMonthDay,
  snippetPlace,
  venueNextShowFrom,
  venueSnippet,
} from './entitySnippets'

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

describe('artistSnippet', () => {
  it('puts the location in the title and the description when there is one', () => {
    const { title, description } = artistSnippet({
      name: 'Headliner Band',
      city: 'Phoenix',
      state: 'AZ',
    })
    expect(title).toBe('Headliner Band · Phoenix, AZ')
    expect(description).toBe(
      'Headliner Band from Phoenix, AZ: shows, similar artists and connections on Psychic Homily'
    )
  })

  it('renders the board row 2 strings for an artist with no location', () => {
    const { title, description } = artistSnippet({
      name: 'Cape Fury',
      city: null,
      state: null,
    })
    expect(title).toBe('Cape Fury')
    expect(description).toBe(
      'Cape Fury: shows, similar artists and connections on Psychic Homily'
    )
  })

  it('names the city alone when there is no state', () => {
    const { title } = artistSnippet({ name: 'Band', city: 'Melbourne', state: null })
    expect(title).toBe('Band · Melbourne')
  })

  it('drops the location from the title when it does not fit', () => {
    const name = 'x'.repeat(35)
    const { title, description } = artistSnippet({
      name,
      city: 'San Francisco',
      state: 'CA',
    })
    expect(title).toBe(name)
    expect(description).toContain('from San Francisco, CA')
  })

  it('keeps the description within budget for a long name', () => {
    const { description } = artistSnippet({
      name: 'n'.repeat(150),
      city: 'Phoenix',
      state: 'AZ',
    })
    expect(Array.from(description)).toHaveLength(DESCRIPTION_BUDGET)
    expect(description.endsWith('...')).toBe(true)
  })
})

describe('venueSnippet', () => {
  const LOST_BAG = { name: 'Lost Bag', city: 'Providence', state: 'RI' }
  const HELENE = {
    headliner: 'Hélène Barbier',
    timing: {
      eventDate: '2026-10-04T00:00:00Z',
      state: 'RI',
      timezone: 'America/New_York',
    },
  }

  it('renders the board row 3 strings with the blessed Next wording', () => {
    const { title, description } = venueSnippet({ ...LOST_BAG, nextShow: HELENE })
    expect(title).toBe('Lost Bag · Providence, RI')
    expect(titleLength(title)).toBe(42)
    expect(description).toBe(
      'Upcoming shows at Lost Bag in Providence, RI. Next: Hélène Barbier, Oct 3.'
    )
  })

  it('marks the Next date when the venue zone is a guess', () => {
    const { description } = venueSnippet({
      name: 'Room',
      city: 'Toronto',
      state: 'ON',
      nextShow: {
        headliner: 'Band',
        timing: { eventDate: '2026-09-12T03:00:00Z', state: 'ON' },
      },
    })
    expect(description).toBe('Upcoming shows at Room in Toronto, ON. Next: Band, ~Sep 11.')
  })

  it('omits the Next clause when there is no next show', () => {
    const { description } = venueSnippet({ ...LOST_BAG, nextShow: null })
    expect(description).toBe('Upcoming shows at Lost Bag in Providence, RI.')
  })

  it('omits the Next clause when the next show has no parseable date', () => {
    const { description } = venueSnippet({
      ...LOST_BAG,
      nextShow: { headliner: 'Band', timing: { eventDate: 'garbage' } },
    })
    expect(description).toBe('Upcoming shows at Lost Bag in Providence, RI.')
  })

  it('omits the place when the venue has none', () => {
    const { title, description } = venueSnippet({
      name: 'Lost Bag',
      city: '',
      state: '',
      nextShow: null,
    })
    expect(title).toBe('Lost Bag')
    expect(description).toBe('Upcoming shows at Lost Bag.')
  })

  it('drops the location from the title when it does not fit', () => {
    const name = 'v'.repeat(30)
    const { title } = venueSnippet({ name, city: 'Providence', state: 'RI' })
    expect(title).toBe(name)
  })

  it('keeps the description within budget for long names', () => {
    const { description } = venueSnippet({
      name: 'v'.repeat(80),
      city: 'Providence',
      state: 'RI',
      nextShow: { ...HELENE, headliner: 'h'.repeat(80) },
    })
    expect(Array.from(description)).toHaveLength(DESCRIPTION_BUDGET)
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
