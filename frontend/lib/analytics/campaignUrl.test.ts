import { describe, it, expect } from 'vitest'
import {
  CAMPAIGN_SOURCES,
  buildCampaignUrl,
  weekCampaignId,
  showCampaignId,
  artistCampaignId,
  weeklyPageCampaignUrl,
  type CampaignSource,
} from './campaignUrl'

const SITE = 'https://psychichomily.com'

describe('weekCampaignId', () => {
  it('joins the scene slug and the week', () => {
    expect(weekCampaignId('chicago-il', '2026-W31')).toBe('week-chicago-il-2026-w31')
  })

  // Analytics breakdowns are case-sensitive, so `2026-W31` and `2026-w31` would
  // split one week into two rows that never get added back together.
  it('lowercases so one week cannot become two rows', () => {
    expect(weekCampaignId('Chicago-IL', '2026-W31')).toBe(
      weekCampaignId('chicago-il', '2026-w31')
    )
  })

  // The same ambiguity the share card had to solve: the bare city is not unique.
  it('keeps same-named cities in different states distinguishable', () => {
    expect(weekCampaignId('portland-or', '2026-W31')).not.toBe(
      weekCampaignId('portland-me', '2026-W31')
    )
  })

  // Trimming the JOINED string would leave a trailing space on the slug sitting
  // in the middle of the result — a second permanent row for the same week.
  it('normalizes each part, not just the ends of the joined string', () => {
    expect(weekCampaignId('chicago-il ', '2026-W31')).toBe('week-chicago-il-2026-w31')
    expect(weekCampaignId(' chicago-il', ' 2026-W31 ')).toBe('week-chicago-il-2026-w31')
  })
})

describe('campaign id namespace', () => {
  // Every id says what kind of thing was posted, so rows stay groupable by
  // artifact type and nobody invents a spelling the first time a show is posted.
  it('type-prefixes each artifact kind', () => {
    expect(weekCampaignId('chicago-il', '2026-W31')).toMatch(/^week-/)
    expect(showCampaignId('ovlov-empty-bottle-2026-07-27')).toBe(
      'show-ovlov-empty-bottle-2026-07-27'
    )
    expect(artistCampaignId('Ovlov')).toBe('artist-ovlov')
  })

  it('keeps the three kinds from ever colliding', () => {
    const ids = [
      weekCampaignId('ovlov', '2026-W31'),
      showCampaignId('ovlov'),
      artistCampaignId('ovlov'),
    ]
    expect(new Set(ids).size).toBe(3)
  })
})

describe('buildCampaignUrl', () => {
  it('adds exactly the two parameters the convention defines', () => {
    const tagged = new URL(buildCampaignUrl(`${SITE}/scenes/chicago-il/2026-W31`, 'bluesky', 'week-chicago-il-2026-w31'))
    expect(tagged.searchParams.get('utm_source')).toBe('bluesky')
    expect(tagged.searchParams.get('utm_campaign')).toBe('week-chicago-il-2026-w31')
    // `utm_medium` was deliberately dropped from the scheme.
    expect(tagged.searchParams.get('utm_medium')).toBeNull()
    expect([...tagged.searchParams.keys()].sort()).toEqual(['utm_campaign', 'utm_source'])
  })

  it('leaves the path and existing query parameters alone', () => {
    const tagged = new URL(buildCampaignUrl(`${SITE}/shows/some-show?ref=x`, 'reddit', 'a-show'))
    expect(tagged.pathname).toBe('/shows/some-show')
    expect(tagged.searchParams.get('ref')).toBe('x')
  })

  // Re-tagging must replace, not append: `?utm_source=a&utm_source=b` is a row
  // in neither breakdown.
  it('is idempotent when a URL is tagged twice', () => {
    const once = buildCampaignUrl(`${SITE}/x`, 'bluesky', 'c1')
    const twice = buildCampaignUrl(once, 'bluesky', 'c1')
    expect(twice).toBe(once)
    expect([...new URL(twice).searchParams.getAll('utm_source')]).toHaveLength(1)
  })

  it('replaces the tags when the same URL is re-posted elsewhere', () => {
    const first = buildCampaignUrl(`${SITE}/x`, 'bluesky', 'c1')
    const second = new URL(buildCampaignUrl(first, 'reddit', 'c2'))
    expect(second.searchParams.get('utm_source')).toBe('reddit')
    expect(second.searchParams.get('utm_campaign')).toBe('c2')
  })

  it('normalizes campaign casing and padding', () => {
    expect(new URL(buildCampaignUrl(`${SITE}/x`, 'bluesky', '  Chicago-IL-2026-W31 ')).searchParams.get('utm_campaign'))
      .toBe('chicago-il-2026-w31')
  })

  // An empty campaign produces traffic that cannot be attributed after the
  // fact, which is unrecoverable — better to fail at the point of posting.
  it('refuses an empty campaign rather than emitting an unattributable link', () => {
    expect(() => buildCampaignUrl(`${SITE}/x`, 'bluesky', '')).toThrow(/campaign/)
    expect(() => buildCampaignUrl(`${SITE}/x`, 'bluesky', '   ')).toThrow(/campaign/)
  })

  // `CampaignSource` is compile-time only. A cast, a CLI argument or a JSON
  // config reaches this at runtime, and `Bluesky` vs `bluesky` would be two
  // permanent rows in every breakdown.
  it('rejects a source outside the closed set, however it arrives', () => {
    for (const bad of ['Bluesky', 'bsky', 'bluesky ', '', 'twitter']) {
      expect(() => buildCampaignUrl(`${SITE}/x`, bad as CampaignSource, 'c')).toThrow(
        /source/
      )
    }
  })

  it('accepts every source in the closed set', () => {
    for (const source of CAMPAIGN_SOURCES) {
      expect(new URL(buildCampaignUrl(`${SITE}/x`, source, 'c')).searchParams.get('utm_source')).toBe(source)
    }
  })

  // The whole point of the optional third field: two posts about the SAME week
  // on the same channel must stay distinguishable.
  it('distinguishes two posts of one artifact when content is given', () => {
    const mon = new URL(buildCampaignUrl(`${SITE}/x`, 'bluesky', 'week-c-2026-w31', 'mon'))
    const thu = new URL(buildCampaignUrl(`${SITE}/x`, 'bluesky', 'week-c-2026-w31', 'thu'))
    expect(mon.searchParams.get('utm_content')).toBe('mon')
    expect(thu.searchParams.get('utm_content')).toBe('thu')
    // Same campaign, so the two are still comparable as one artifact.
    expect(mon.searchParams.get('utm_campaign')).toBe(
      thu.searchParams.get('utm_campaign')
    )
  })

  it('omits utm_content entirely when there is only one post', () => {
    const url = new URL(buildCampaignUrl(`${SITE}/x`, 'bluesky', 'c'))
    expect(url.searchParams.has('utm_content')).toBe(false)
  })

  // A stale content tag inherited from a previously-tagged URL would silently
  // attribute a new post to the old placement.
  it('clears a stale utm_content when re-tagged without one', () => {
    const withContent = buildCampaignUrl(`${SITE}/x`, 'bluesky', 'c', 'mon')
    const without = new URL(buildCampaignUrl(withContent, 'bluesky', 'c'))
    expect(without.searchParams.has('utm_content')).toBe(false)
  })

  it('normalizes content, and refuses it blank rather than emitting a junk row', () => {
    expect(
      new URL(buildCampaignUrl(`${SITE}/x`, 'bluesky', 'c', ' R-Chicago ')).searchParams.get('utm_content')
    ).toBe('r-chicago')
    expect(() => buildCampaignUrl(`${SITE}/x`, 'bluesky', 'c', '  ')).toThrow(/content/)
  })

  // `new URL()` is not URL validation: it accepts a typo'd scheme and rejects
  // the root-relative form a caller is most likely to reach for.
  it('rejects anything that is not an absolute https URL', () => {
    for (const bad of [
      'not-a-url',
      'htp://psychichomily.com/x',
      'http://psychichomily.com/x',
      'javascript:alert(1)',
      'mailto:a@b.com',
      '/scenes/chicago-il/2026-W31',
    ]) {
      expect(() => buildCampaignUrl(bad, 'bluesky', 'c'), bad).toThrow()
    }
  })
})

describe('weeklyPageCampaignUrl', () => {
  // Points at the archived permalink, not the rolling /week URL: a post is
  // about one specific week, and /week would show a different one to anyone
  // following the link later.
  it('targets the archived permalink, not the rolling week', () => {
    const url = new URL(weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W31', 'bluesky'))
    expect(url.pathname).toBe('/scenes/chicago-il/2026-W31')
    expect(url.pathname).not.toContain('/week')
  })

  it('derives the campaign from the scene and week', () => {
    const url = new URL(weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W31', 'bluesky'))
    expect(url.searchParams.get('utm_campaign')).toBe('week-chicago-il-2026-w31')
    expect(url.searchParams.get('utm_source')).toBe('bluesky')
  })

  it('does not double the slash however many the origin has', () => {
    for (const origin of [`${SITE}/`, `${SITE}//`]) {
      expect(weeklyPageCampaignUrl(origin, 'chicago-il', '2026-W31', 'bluesky')).toBe(
        weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W31', 'bluesky')
      )
    }
  })

  // These feed the PATH, so a bad value is a publicly posted 404 that still
  // logs a confident-looking campaign row.
  it('rejects anything that is not a scene slug', () => {
    for (const bad of ['san diego', 'Chicago-IL', 'chi#frag', '..', 'chicago_il', '']) {
      expect(() => weeklyPageCampaignUrl(SITE, bad, '2026-W31', 'bluesky'), bad).toThrow(
        /slug/
      )
    }
  })

  // `2026-W1` is what `${year}-W${week}` gives you without padStart; the route
  // 404s on it, and it is an unmergeable second row for the same week.
  it('requires a zero-padded week key', () => {
    expect(() => weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W1', 'bluesky')).toThrow(
      /ISO week/
    )
    expect(() => weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-31', 'bluesky')).toThrow()
  })

  // The route matches the week case-insensitively, so a lowercase spelling
  // renders 200 at a non-canonical URL and splits the top-pages row in two.
  it('emits the canonical uppercase week in the path', () => {
    const url = new URL(weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-w31', 'bluesky'))
    expect(url.pathname).toBe('/scenes/chicago-il/2026-W31')
    expect(url.searchParams.get('utm_campaign')).toBe('week-chicago-il-2026-w31')
  })

  it('produces a distinct campaign per week and per source', () => {
    const w31 = new URL(weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W31', 'bluesky'))
    const w32 = new URL(weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W32', 'bluesky'))
    const reddit = new URL(weeklyPageCampaignUrl(SITE, 'chicago-il', '2026-W31', 'reddit'))
    expect(w31.searchParams.get('utm_campaign')).not.toBe(w32.searchParams.get('utm_campaign'))
    expect(w31.searchParams.get('utm_source')).not.toBe(reddit.searchParams.get('utm_source'))
    // Same post, different channel — the campaign must match so the two rows
    // can be compared against each other.
    expect(w31.searchParams.get('utm_campaign')).toBe(reddit.searchParams.get('utm_campaign'))
  })
})

describe('CAMPAIGN_SOURCES', () => {
  // A closed set is the point: `bluesky`, `Bluesky` and `bsky` would be three
  // separate rows in every breakdown, permanently.
  it('is lowercase and unique', () => {
    expect(CAMPAIGN_SOURCES).toEqual([...new Set(CAMPAIGN_SOURCES)])
    for (const source of CAMPAIGN_SOURCES) {
      expect(source).toBe(source.toLowerCase())
      expect(source).not.toMatch(/\s/)
    }
  })

  it('covers the channels the distribution push names', () => {
    for (const channel of ['bluesky', 'reddit', 'instagram', 'substack']) {
      expect(CAMPAIGN_SOURCES).toContain(channel)
    }
  })
})
