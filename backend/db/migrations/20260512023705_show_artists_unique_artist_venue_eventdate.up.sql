CREATE UNIQUE INDEX CONCURRENTLY shows_artist_venue_eventdate_uniq
  ON show_artists (artist_id, venue_id, event_date)
  WHERE event_date IS NOT NULL AND venue_id IS NOT NULL;
