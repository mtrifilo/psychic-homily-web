import { Database } from 'bun:sqlite'
import { mkdirSync } from 'fs'
import { join, dirname } from 'path'

const DB_PATH = join(import.meta.dir, '../../data/discovery.db')

let db: Database

function getDb(): Database {
  if (!db) {
    mkdirSync(dirname(DB_PATH), { recursive: true })
    db = new Database(DB_PATH)
    db.exec('PRAGMA journal_mode = WAL')
    db.exec('PRAGMA foreign_keys = ON')
    initSchema()
  }
  return db
}

function initSchema() {
  const d = getDb()
  d.exec(`
    CREATE TABLE IF NOT EXISTS scrape_sessions (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      venue_slug TEXT NOT NULL,
      scrape_type TEXT NOT NULL CHECK (scrape_type IN ('preview', 'full')),
      event_count INTEGER NOT NULL DEFAULT 0,
      created_at TEXT NOT NULL DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS events (
      venue_slug TEXT NOT NULL,
      event_id TEXT NOT NULL,
      title TEXT NOT NULL,
      date TEXT NOT NULL,
      first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
      last_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
      is_ignored INTEGER NOT NULL DEFAULT 0,
      ignored_at TEXT,
      PRIMARY KEY (venue_slug, event_id)
    );

    CREATE TABLE IF NOT EXISTS event_snapshots (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      session_id INTEGER NOT NULL REFERENCES scrape_sessions(id),
      venue_slug TEXT NOT NULL,
      event_id TEXT NOT NULL,
      title TEXT NOT NULL,
      date TEXT NOT NULL,
      artists TEXT,
      price TEXT,
      is_sold_out INTEGER NOT NULL DEFAULT 0,
      is_cancelled INTEGER NOT NULL DEFAULT 0,
      scraped_at TEXT NOT NULL DEFAULT (datetime('now'))
    );
  `)
}

// ---- Preview recording ----

interface PreviewEventInput {
  id: string
  title: string
  date: string
}

export function recordPreviewEvents(
  venueSlug: string,
  events: PreviewEventInput[],
): { newEventIds: string[] } {
  const d = getDb()
  const now = new Date().toISOString()

  // Record session
  const sessionStmt = d.prepare(
    'INSERT INTO scrape_sessions (venue_slug, scrape_type, event_count, created_at) VALUES (?, ?, ?, ?)',
  )
  sessionStmt.run(venueSlug, 'preview', events.length, now)

  // Upsert events
  const upsertStmt = d.prepare(`
    INSERT INTO events (venue_slug, event_id, title, date, first_seen_at, last_seen_at)
    VALUES (?, ?, ?, ?, ?, ?)
    ON CONFLICT (venue_slug, event_id) DO UPDATE SET
      title = excluded.title,
      date = excluded.date,
      last_seen_at = excluded.last_seen_at
  `)

  const existingStmt = d.prepare(
    'SELECT event_id FROM events WHERE venue_slug = ? AND event_id = ?',
  )

  const newEventIds: string[] = []

  const transaction = d.transaction(() => {
    for (const event of events) {
      const existing = existingStmt.get(venueSlug, event.id) as { event_id: string } | null
      if (!existing) {
        newEventIds.push(event.id)
      }
      upsertStmt.run(venueSlug, event.id, event.title, event.date, now, now)
    }
  })
  transaction()

  return { newEventIds }
}

// ---- Full scrape recording ----

interface ScrapeEventInput {
  id: string
  title: string
  date: string
  artists?: string[]
  price?: string
  isSoldOut?: boolean
  isCancelled?: boolean
}

interface FieldChange {
  field: 'isSoldOut' | 'isCancelled' | 'price' | 'date' | 'title'
  oldValue: string | boolean | null
  newValue: string | boolean | null
}

export function recordScrapeResults(
  venueSlug: string,
  events: ScrapeEventInput[],
): { changes: Record<string, FieldChange[]> } {
  const d = getDb()
  const now = new Date().toISOString()

  // Record session
  const sessionStmt = d.prepare(
    'INSERT INTO scrape_sessions (venue_slug, scrape_type, event_count, created_at) VALUES (?, ?, ?, ?) RETURNING id',
  )
  const session = sessionStmt.get(venueSlug, 'full', events.length, now) as { id: number }

  // Get previous snapshots for change detection
  const prevSnapshotsStmt = d.prepare(`
    SELECT es.event_id, es.title, es.date, es.price, es.is_sold_out, es.is_cancelled
    FROM event_snapshots es
    INNER JOIN (
      SELECT event_id, MAX(id) as max_id
      FROM event_snapshots
      WHERE venue_slug = ?
      GROUP BY event_id
    ) latest ON es.id = latest.max_id
  `)
  const prevSnapshots = prevSnapshotsStmt.all(venueSlug) as Array<{
    event_id: string
    title: string
    date: string
    price: string | null
    is_sold_out: number
    is_cancelled: number
  }>
  const prevMap = new Map(prevSnapshots.map(s => [s.event_id, s]))

  // Insert new snapshots
  const snapshotStmt = d.prepare(`
    INSERT INTO event_snapshots (session_id, venue_slug, event_id, title, date, artists, price, is_sold_out, is_cancelled, scraped_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `)

  // Upsert into events table too
  const upsertStmt = d.prepare(`
    INSERT INTO events (venue_slug, event_id, title, date, first_seen_at, last_seen_at)
    VALUES (?, ?, ?, ?, ?, ?)
    ON CONFLICT (venue_slug, event_id) DO UPDATE SET
      title = excluded.title,
      date = excluded.date,
      last_seen_at = excluded.last_seen_at
  `)

  const changes: Record<string, FieldChange[]> = {}

  const transaction = d.transaction(() => {
    for (const event of events) {
      snapshotStmt.run(
        session.id,
        venueSlug,
        event.id,
        event.title,
        event.date,
        event.artists ? JSON.stringify(event.artists) : null,
        event.price ?? null,
        event.isSoldOut ? 1 : 0,
        event.isCancelled ? 1 : 0,
        now,
      )

      upsertStmt.run(venueSlug, event.id, event.title, event.date, now, now)

      // Detect changes from previous snapshot
      const prev = prevMap.get(event.id)
      if (prev) {
        const eventChanges: FieldChange[] = []

        if (prev.title !== event.title) {
          eventChanges.push({ field: 'title', oldValue: prev.title, newValue: event.title })
        }
        if (prev.date !== event.date) {
          eventChanges.push({ field: 'date', oldValue: prev.date, newValue: event.date })
        }
        if ((prev.price ?? null) !== (event.price ?? null)) {
          eventChanges.push({ field: 'price', oldValue: prev.price, newValue: event.price ?? null })
        }
        if (Boolean(prev.is_sold_out) !== Boolean(event.isSoldOut)) {
          eventChanges.push({ field: 'isSoldOut', oldValue: Boolean(prev.is_sold_out), newValue: Boolean(event.isSoldOut) })
        }
        if (Boolean(prev.is_cancelled) !== Boolean(event.isCancelled)) {
          eventChanges.push({ field: 'isCancelled', oldValue: Boolean(prev.is_cancelled), newValue: Boolean(event.isCancelled) })
        }

        if (eventChanges.length > 0) {
          changes[event.id] = eventChanges
        }
      }
    }
  })
  transaction()

  return { changes }
}

// ---- Ignore toggle ----

export function setEventIgnored(
  venueSlug: string,
  eventId: string,
  ignored: boolean,
): void {
  const d = getDb()
  const now = new Date().toISOString()
  d.prepare(
    'UPDATE events SET is_ignored = ?, ignored_at = ? WHERE venue_slug = ? AND event_id = ?',
  ).run(ignored ? 1 : 0, ignored ? now : null, venueSlug, eventId)
}

// ---- Metadata retrieval ----

interface EventMetadata {
  isNew: boolean
  isIgnored: boolean
  firstSeenAt: string
  lastSeenAt: string
  changes: FieldChange[]
}

export function getEventMetadata(
  venueSlug: string,
): Record<string, EventMetadata> {
  const d = getDb()

  // Get the timestamp of the second-most-recent preview session for this venue.
  // Events first seen after that timestamp are "new".
  const prevSessionStmt = d.prepare(`
    SELECT created_at FROM scrape_sessions
    WHERE venue_slug = ? AND scrape_type = 'preview'
    ORDER BY id DESC
    LIMIT 1 OFFSET 1
  `)
  const prevSession = prevSessionStmt.get(venueSlug) as { created_at: string } | null
  const newThreshold = prevSession?.created_at ?? null

  // Get all events for this venue
  const eventsStmt = d.prepare(
    'SELECT event_id, title, date, first_seen_at, last_seen_at, is_ignored FROM events WHERE venue_slug = ?',
  )
  const events = eventsStmt.all(venueSlug) as Array<{
    event_id: string
    title: string
    date: string
    first_seen_at: string
    last_seen_at: string
    is_ignored: number
  }>

  // Get latest changes from snapshots (compare last two snapshots per event)
  const changesStmt = d.prepare(`
    SELECT es1.event_id,
           es1.title as new_title, es1.date as new_date, es1.price as new_price,
           es1.is_sold_out as new_sold_out, es1.is_cancelled as new_cancelled,
           es2.title as old_title, es2.date as old_date, es2.price as old_price,
           es2.is_sold_out as old_sold_out, es2.is_cancelled as old_cancelled
    FROM event_snapshots es1
    INNER JOIN event_snapshots es2 ON es1.venue_slug = es2.venue_slug AND es1.event_id = es2.event_id
    WHERE es1.venue_slug = ?
      AND es1.id = (
        SELECT MAX(id) FROM event_snapshots
        WHERE venue_slug = es1.venue_slug AND event_id = es1.event_id
      )
      AND es2.id = (
        SELECT MAX(id) FROM event_snapshots
        WHERE venue_slug = es1.venue_slug AND event_id = es1.event_id AND id < es1.id
      )
  `)
  const snapshotChanges = changesStmt.all(venueSlug) as Array<{
    event_id: string
    new_title: string; new_date: string; new_price: string | null; new_sold_out: number; new_cancelled: number
    old_title: string; old_date: string; old_price: string | null; old_sold_out: number; old_cancelled: number
  }>

  const changeMap = new Map<string, FieldChange[]>()
  for (const row of snapshotChanges) {
    const changes: FieldChange[] = []
    if (row.old_title !== row.new_title) {
      changes.push({ field: 'title', oldValue: row.old_title, newValue: row.new_title })
    }
    if (row.old_date !== row.new_date) {
      changes.push({ field: 'date', oldValue: row.old_date, newValue: row.new_date })
    }
    if (row.old_price !== row.new_price) {
      changes.push({ field: 'price', oldValue: row.old_price, newValue: row.new_price })
    }
    if (Boolean(row.old_sold_out) !== Boolean(row.new_sold_out)) {
      changes.push({ field: 'isSoldOut', oldValue: Boolean(row.old_sold_out), newValue: Boolean(row.new_sold_out) })
    }
    if (Boolean(row.old_cancelled) !== Boolean(row.new_cancelled)) {
      changes.push({ field: 'isCancelled', oldValue: Boolean(row.old_cancelled), newValue: Boolean(row.new_cancelled) })
    }
    if (changes.length > 0) {
      changeMap.set(row.event_id, changes)
    }
  }

  const result: Record<string, EventMetadata> = {}
  for (const event of events) {
    const isNew = newThreshold !== null && event.first_seen_at > newThreshold
    result[event.event_id] = {
      isNew,
      isIgnored: event.is_ignored === 1,
      firstSeenAt: event.first_seen_at,
      lastSeenAt: event.last_seen_at,
      changes: changeMap.get(event.event_id) ?? [],
    }
  }

  return result
}

// ---- Last scrape info ----

export function getLastScrapeInfo(
  venueSlug: string,
): { lastScrapeAt: string; eventCount: number } | null {
  const d = getDb()
  const stmt = d.prepare(`
    SELECT created_at, event_count FROM scrape_sessions
    WHERE venue_slug = ?
    ORDER BY id DESC
    LIMIT 1
  `)
  const row = stmt.get(venueSlug) as { created_at: string; event_count: number } | null
  if (!row) return null
  return { lastScrapeAt: row.created_at, eventCount: row.event_count }
}
