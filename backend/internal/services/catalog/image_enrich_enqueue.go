package catalog

import (
	"log/slog"
	"os"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// imageEnrichEnabled gates the image-enrichment outbox on the same opt-in as the
// Phase-A sweep + the poller (ENABLE_IMAGE_ENRICH_SWEEP). When the feature is off
// (the current pre-prod default — display is gated on PSY-1242 and nothing drains
// the queue) we must NOT enqueue: an ungated enqueue would write a `pending` row
// per create that no poller ever drains and no prune ever removes, growing the
// table unboundedly. With the gate, the feature is fully dormant when off (no
// rows) and self-bounding when on (poller drains, prune reaps terminal rows).
func imageEnrichEnabled() bool {
	return os.Getenv("ENABLE_IMAGE_ENRICH_SWEEP") == "1"
}

// enqueueImageEnrich writes a best-effort image-enrichment outbox job for a
// freshly-created entity, on the caller's transaction (PSY-1247). It is called
// from the single artist create funnel (FindOrCreateArtistTx, created path only)
// and from ReleaseService.CreateRelease, so every create path enqueues without any
// per-site bookkeeping. No-ops when the feature is disabled (see imageEnrichEnabled).
//
// Atomicity depends on how the caller calls the funnel:
//
//   - Inside a caller transaction (CreateRelease; show-import / discovery, which
//     pass their tx): the insert runs on that tx, so it is ATOMIC with the create —
//     if the create rolls back, the job row does too (no orphan).
//   - On the base *gorm.DB (admin CreateArtist, data-sync, seed, festival-entry):
//     the create already auto-committed in its own statement, so the enqueue is a
//     separate best-effort write — there is no surrounding tx to be atomic with.
//     Worst case is an artist with no job, which the Phase-A sweep backfills.
//
// EITHER WAY the enqueue NEVER fails the entity create. That is the non-obvious
// part: in Postgres ANY failed statement aborts the WHOLE transaction, so when the
// funnel IS inside a caller tx, a naive failing insert would poison it and the
// later COMMIT would silently become a ROLLBACK — the create would vanish and the
// caller would see a phantom success (non-nil entity, nil error, no DB row). The
// fix is a SAVEPOINT: the insert runs in a nested transaction (GORM emits SAVEPOINT
// / ROLLBACK TO SAVEPOINT when the receiver is already a tx), so a failed insert
// rolls back ONLY the savepoint and leaves the outer tx healthy to commit the
// entity. On the base *gorm.DB the nested call is a standalone tx — still isolating
// the failure.
//
// ON CONFLICT DO NOTHING makes a re-enqueue a no-op against the
// one-active-job-per-entity partial unique index (uq_image_enrich_queue_active),
// so the common "already queued" case is not even an error.
func enqueueImageEnrich(tx *gorm.DB, entityType string, entityID uint) {
	if !imageEnrichEnabled() {
		return
	}
	item := &catalogm.ImageEnrichQueueItem{
		EntityType: entityType,
		EntityID:   entityID,
		Status:     catalogm.ImageEnrichStatusPending,
	}
	err := tx.Transaction(func(itx *gorm.DB) error {
		return itx.Clauses(clause.OnConflict{DoNothing: true}).Create(item).Error
	})
	if err != nil {
		slog.Default().Warn("image-enrich enqueue failed (entity create unaffected)",
			"entity_type", entityType, "entity_id", entityID, "error", err)
	}
}
