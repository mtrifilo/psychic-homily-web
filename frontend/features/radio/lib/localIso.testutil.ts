/**
 * TEST-ONLY helper for the PSY-1298 viewer-local rendering tests: build an
 * ISO instant FROM local-time Date components so expectations hold in any
 * machine timezone (the code under test renders in the viewer's local zone
 * by design). NOTE the month is zero-indexed — `localIso(2026, 5, 9, 15)`
 * is June 9, 3 PM local. Keep every windowed-fixture test on this helper;
 * hand-rolled copies are exactly how a wrong month index ships a test that
 * passes in one timezone and fails in another.
 */
export function localIso(
  year: number,
  monthIndex: number,
  day: number,
  hour: number,
  minute = 0
): string {
  return new Date(year, monthIndex, day, hour, minute).toISOString()
}
