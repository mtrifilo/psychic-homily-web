import { describe, it, expect } from 'vitest'
import * as hooks from './index'

// Low-cost guard against accidental rename/delete in the barrel: assert each
// named hook re-export resolves to a callable function. Grouped by source
// module so a missing module surfaces an obvious cluster of failures.
// Type-only re-exports (ExportShowData, VenueMatchResult, ArtistMatchResult,
// ImportPreviewResponse, ShowSubmission, ShowUpdate*, ShowUpdateResponse) have
// no runtime presence and are not asserted.
describe('shows hooks barrel', () => {
  it('re-exports the useShows read hooks', () => {
    expect(typeof hooks.useUpcomingShows).toBe('function')
    expect(typeof hooks.useShow).toBe('function')
    expect(typeof hooks.useShowCities).toBe('function')
  })

  it('re-exports the delete / extraction hooks', () => {
    expect(typeof hooks.useShowDelete).toBe('function')
    expect(typeof hooks.useShowExtraction).toBe('function')
  })

  it('re-exports the import hooks', () => {
    expect(typeof hooks.useShowImportPreview).toBe('function')
    expect(typeof hooks.useShowImportConfirm).toBe('function')
  })

  it('re-exports the visibility / reminder hooks', () => {
    expect(typeof hooks.useShowMakePrivate).toBe('function')
    expect(typeof hooks.useShowPublish).toBe('function')
    expect(typeof hooks.useSetShowReminders).toBe('function')
    expect(typeof hooks.useShowUnpublish).toBe('function')
  })

  it('re-exports the report hooks', () => {
    expect(typeof hooks.useMyShowReport).toBe('function')
    expect(typeof hooks.useReportShow).toBe('function')
  })

  it('re-exports the submit / update hooks', () => {
    expect(typeof hooks.useShowSubmit).toBe('function')
    expect(typeof hooks.useShowUpdate).toBe('function')
    expect(typeof hooks.useMySubmissions).toBe('function')
  })

  it('re-exports the saved-shows hooks', () => {
    expect(typeof hooks.useSavedShows).toBe('function')
    expect(typeof hooks.useSaveShow).toBe('function')
    expect(typeof hooks.useUnsaveShow).toBe('function')
    expect(typeof hooks.useSaveShowToggle).toBe('function')
    expect(typeof hooks.useShowSaveCount).toBe('function')
    expect(typeof hooks.useShowSaveCountBatch).toBe('function')
  })
})
