import { describe, it, expect, vi, beforeEach } from 'vitest'

const captureMessage = vi.fn()
vi.mock('@sentry/nextjs', () => ({
  captureMessage: (...args: unknown[]) => captureMessage(...args),
}))

import {
  reportPlantedTicketTag,
  resetPlantedTagReportsForTest,
} from './plantedTagTelemetry'

const TAG = { param: 'irmp', host: 'www.ticketweb.com' }

beforeEach(() => {
  captureMessage.mockClear()
  resetPlantedTagReportsForTest()
})

describe('reportPlantedTicketTag', () => {
  it('reports a warning naming the host and the parameter', () => {
    reportPlantedTicketTag({ entityType: 'show', entityId: 42, tag: TAG })

    expect(captureMessage).toHaveBeenCalledTimes(1)
    const [message, options] = captureMessage.mock.calls[0] as [
      string,
      { level: string; tags: Record<string, unknown>; extra: Record<string, unknown> },
    ]
    expect(message).toMatch(/planted/i)
    expect(options.level).toBe('warning')
    expect(options.tags).toMatchObject({
      error_type: 'planted_affiliate_tag',
      entity_type: 'show',
      ticket_host: 'www.ticketweb.com',
      affiliate_param: 'irmp',
    })
    expect(options.extra).toMatchObject({ entityId: 42 })
  })

  // The partner ID is a third party's account identifier and the rest of the
  // URL is contributor text. Neither may leave the process.
  it('ships no partner ID, path, query or full URL', () => {
    reportPlantedTicketTag({ entityType: 'show', entityId: 42, tag: TAG })

    const serialized = JSON.stringify(captureMessage.mock.calls[0])
    expect(serialized).not.toContain('9999999')
    expect(serialized).not.toContain('/e/2')
    expect(serialized).not.toMatch(/https?:\/\//)
  })

  // A planted tag is a property of a stored row, so every viewer and every
  // re-render sees the same one. Per-occurrence reporting would turn one bad
  // row into unbounded identical events.
  it('reports a given row and tag only once per process', () => {
    for (let i = 0; i < 5; i++) {
      reportPlantedTicketTag({ entityType: 'show', entityId: 42, tag: TAG })
    }
    expect(captureMessage).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['a different entity', { entityType: 'show' as const, entityId: 43, tag: TAG }],
    [
      'a different entity type',
      { entityType: 'festival' as const, entityId: 42, tag: TAG },
    ],
    [
      'a different host',
      { entityType: 'show' as const, entityId: 42, tag: { ...TAG, host: 'dice.fm' } },
    ],
  ])('still reports %s', (_label, second) => {
    reportPlantedTicketTag({ entityType: 'show', entityId: 42, tag: TAG })
    reportPlantedTicketTag(second)
    expect(captureMessage).toHaveBeenCalledTimes(2)
  })

  // This runs from a render effect on a page whose job is showing a reader
  // where to buy a ticket. A telemetry fault must never become that page's
  // problem.
  it('never throws when Sentry does', () => {
    captureMessage.mockImplementationOnce(() => {
      throw new Error('transport down')
    })
    expect(() =>
      reportPlantedTicketTag({ entityType: 'show', entityId: 42, tag: TAG })
    ).not.toThrow()
  })
})
