import { describe, it, expect } from 'vitest'
import { actionsFootprintFor } from './showListActionsFootprint'
import type { ShowResponse } from '../types'

function show(id: number, submittedBy?: number): ShowResponse {
  return { id, submitted_by: submittedBy } as ShowResponse
}

describe('actionsFootprintFor', () => {
  it('is viewer for an anonymous reader', () => {
    expect(
      actionsFootprintFor({ shows: [show(1, 42)], isAdmin: false })
    ).toBe('viewer')
  })

  it('is viewer for a signed-in reader who submitted none of the rows', () => {
    expect(
      actionsFootprintFor({
        shows: [show(1, 7), show(2)],
        isAdmin: false,
        userId: '42',
      })
    ).toBe('viewer')
  })

  // One owned row widens the column for the whole list, header included:
  // sizing only that row is what moves its venue and price.
  it('is owner when the reader submitted any row on the list', () => {
    expect(
      actionsFootprintFor({
        shows: [show(1, 7), show(2, 42), show(3)],
        isAdmin: false,
        userId: '42',
      })
    ).toBe('owner')
  })

  it('is admin for an admin, whoever submitted the rows', () => {
    expect(
      actionsFootprintFor({ shows: [show(1)], isAdmin: true, userId: '1' })
    ).toBe('admin')
  })

  it('is admin for an admin even on an empty list, so the header agrees', () => {
    expect(actionsFootprintFor({ shows: [], isAdmin: true })).toBe('admin')
  })
})
