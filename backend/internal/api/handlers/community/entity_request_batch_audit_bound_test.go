package community

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// auditWriteConcurrencyCeiling is the worker count the shared audit writer runs.
// It is restated here rather than read from the writer: a test that read the live
// value would pass whatever that value became, including one that bounds nothing.
const auditWriteConcurrencyCeiling = 4

// A full batch files one audit row per item, and this route is the widest
// fan-out in the codebase: 200 items in one request. The bound asserted is on
// CONCURRENT audit writes rather than on how many run, because each one holds a
// database connection while it runs and the pool is shared with the requests
// still being served.
//
// The handler is driven with mocks rather than a database: the writer is what is
// under test, and a real INSERT would make the observed concurrency a property
// of Postgres timing instead of the worker count.
func TestCreateEntityRequestBatch_AuditWritesAreBounded(t *testing.T) {
	const items = 200

	var (
		mu      sync.Mutex
		live    int
		maxLive int
		ran     atomic.Int64
	)

	names := make([]string, items)
	for i := range names {
		names[i] = "Bounded Act " + strconv.Itoa(i)
	}

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(_ *authm.User, _ string, _ []byte, _ string, _ []byte, _ bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
				return pendingRequest(7, "artist"), nil, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{
			LogActionFn: func(_ uint, _ string, _ string, _ uint, _ map[string]interface{}) {
				mu.Lock()
				live++
				if live > maxLive {
					maxLive = live
				}
				mu.Unlock()

				// Long enough that a fan-out overlaps here rather than slipping
				// past one write at a time.
				time.Sleep(2 * time.Millisecond)

				mu.Lock()
				live--
				mu.Unlock()
				ran.Add(1)
			},
		},
	)

	if _, err := h.CreateEntityRequestBatchHandler(erUserCtx(), batchRequest(t, names...)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The writes are queued, so wait for them rather than reading the handler's
	// return as meaning they are done.
	deadline := time.Now().Add(15 * time.Second)
	for ran.Load() < items && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if got := ran.Load(); got != items {
		t.Fatalf("expected %d audit writes to run, got %d (dropped: %d)",
			items, got, servicesshared.DroppedAuditWrites())
	}

	mu.Lock()
	peak := maxLive
	mu.Unlock()
	if peak > auditWriteConcurrencyCeiling {
		t.Errorf("a %d-item batch put %d audit writes in flight at once; the bound is %d",
			items, peak, auditWriteConcurrencyCeiling)
	}
	if peak < 2 {
		t.Errorf("peak concurrency was %d, so this would pass against a fully serial writer and proves nothing", peak)
	}
}
