# Discovery App: Bug Fixes, Cleanup, and Edge Case Hardening

## Phase 1: Bug Fixes

- [x] **1.0** Create shared `getLocalDateString()` helper (`discovery/src/lib/dates.ts`)
- [x] **1.1** Fix `selectAllEvents` selecting past events — filter to `e.date >= today` using local date
- [x] **1.2** Fix auto-preview not re-firing after venue change — track per-slug instead of boolean
- [x] **1.3** Deduplicate scraped events — filter by existing IDs before appending

## Phase 2: Dead Code Cleanup

- [x] **2.1** Delete `EventSelector.tsx`
- [x] **2.2** Remove `'select'` from `WizardStep` type and Header nav array

## Phase 3: Edge Case Hardening

- [x] **3.1** Replace `toISOString().split('T')[0]` with `getLocalDateString()` in EventList.tsx and VenuePreviewCard.tsx
- [x] **3.2** Fix `extractTime` timezone handling in JSON-LD provider — regex parse instead of `new Date().getHours()`

## Verification

1. Run `bun run dev` from `/discovery` — confirm no build errors
2. Select venues, click Preview Events — auto-preview should fire immediately
3. Go back to venues, change selection, return to preview — auto-preview should fire again for new venues
4. Click "All" on a venue — should only select future events, count should match visible events
5. Scrape events, go back, scrape same events again — import step should not show duplicates
6. Preview The Van Buren — times should show correct local times (e.g., 8:00 PM not 3:00 AM)
