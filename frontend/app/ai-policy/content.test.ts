import { describe, it, expect } from 'vitest'
import {
  AI_POLICY_COPY,
  AI_POLICY_PATH,
  isAiPolicyCopyPending,
  type AiPolicyCopy,
} from './content'

// Fixture prose only. Never mistake this for policy copy — the real copy is
// owner-written and lives in AI_POLICY_COPY.
function filled(overrides: Partial<AiPolicyCopy> = {}): AiPolicyCopy {
  return {
    description: 'FIXTURE description.',
    intro: ['FIXTURE intro.'],
    lastUpdated: null,
    sections: [{ id: 'a', heading: 'A', body: ['FIXTURE body.'] }],
    ...overrides,
  }
}

describe('isAiPolicyCopyPending', () => {
  it('reports fully-written copy as published', () => {
    expect(isAiPolicyCopyPending(filled())).toBe(false)
  })

  it.each([
    ['description', filled({ description: null })],
    ['intro', filled({ intro: null })],
    [
      'a section body',
      filled({ sections: [{ id: 'a', heading: 'A', body: null }] }),
    ],
  ])('fails closed when %s is unwritten', (_slot, copy) => {
    expect(isAiPolicyCopyPending(copy)).toBe(true)
  })

  // A section appended without copy must hold the whole page back rather than
  // publishing a page with one silently empty heading.
  it('fails closed when only one of several sections is unwritten', () => {
    const copy = filled({
      sections: [
        { id: 'a', heading: 'A', body: ['FIXTURE body.'] },
        { id: 'b', heading: 'B', body: null },
      ],
    })

    expect(isAiPolicyCopyPending(copy)).toBe(true)
  })

  it('lastUpdated is not part of the gate', () => {
    expect(isAiPolicyCopyPending(filled({ lastUpdated: null }))).toBe(false)
    expect(isAiPolicyCopyPending(filled({ lastUpdated: 'July 31, 2026' }))).toBe(
      false
    )
  })
})

describe('AI_POLICY_COPY', () => {
  // Deliberately no assertion on the section COUNT or on the headings: those
  // are the owner's to rename, merge, or reorder along with the prose. What
  // must hold in any arrangement is that each section is separately
  // addressable, and that the path the footer and sitemap key off is stable.
  it('gives every section a unique anchor id so it can be quoted on its own', () => {
    const ids = AI_POLICY_COPY.sections.map(s => s.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(ids.every(id => id.length > 0)).toBe(true)
  })

  it('pins the canonical path — the footer and sitemap both key off it', () => {
    expect(AI_POLICY_PATH).toBe('/ai-policy')
  })
})
