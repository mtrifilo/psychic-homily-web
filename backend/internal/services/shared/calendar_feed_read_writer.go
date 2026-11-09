package shared

import (
	"context"
	"sync"
	"sync/atomic"
)

// CalendarFeedReadWorkers is how many calendar feed-read records may be in the
// database at once. Each is one conditional upsert, recorded after the feed
// response it describes, so a small number keeps polls (whose routes are exempt
// from the public-read limiter) from claiming the connection pool. Exported so
// the composition root can compare the background workers with the pool
// ceiling at boot.
const CalendarFeedReadWorkers = 2

// calendarFeedReadQueueDepth is how many feed-read records may be waiting. A
// dropped record costs one observation: the client's next poll records the
// read instead.
const calendarFeedReadQueueDepth = 64

// calendarFeedReadWriter is the process-wide bounded writer for feed-read
// records, built on first use like auditWriter and for the same reasons.
var (
	calendarFeedReadWriterStarted atomic.Bool
	calendarFeedReadWriter        = sync.OnceValue(func() *boundedWriter {
		calendarFeedReadWriterStarted.Store(true)
		return newBoundedWriter("calendar_feed_read", CalendarFeedReadWorkers, calendarFeedReadQueueDepth)
	})
)

// SubmitCalendarFeedReadWrite queues a feed-read record on its bounded writer
// and reports whether it was accepted; it never blocks. The work must bound its
// own duration (see boundedWriter's loss policy).
func SubmitCalendarFeedReadWrite(work func()) bool {
	return calendarFeedReadWriter().submit("calendar_feed_read", work)
}

// ShutdownCalendarFeedReadWrites drains the queued feed-read records, under the
// same contract as ShutdownAuditWrites.
func ShutdownCalendarFeedReadWrites(ctx context.Context) error {
	if !calendarFeedReadWriterStarted.Load() {
		return nil
	}
	return calendarFeedReadWriter().shutdown(ctx)
}
