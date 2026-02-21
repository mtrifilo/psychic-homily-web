// Strips event-type parenthetical suffixes from artist names
// e.g. "Wallplant (Album Release)" -> "Wallplant"
export function cleanArtistName(name: string): string {
  return name
    .replace(/\s*\((?:album|record|ep|single|cd|lp|vinyl)\s+release(?:\s+(?:show|party))?\)/i, '')
    .replace(/\s*\(release\s+(?:show|party)\)/i, '')
    .replace(/\s*\((?:farewell|reunion|hometown|record release)\s+show\)/i, '')
    .trim()
}
