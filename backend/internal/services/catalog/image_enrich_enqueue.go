package catalog

import (
	"log/slog"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// enqueueImageEnrich writes a best-effort image-enrichment outbox job for a
// freshly-created entity, on the caller's transaction (PSY-1247). It is called
// from the single artist create funnel (FindOrCreateArtistTx, created path only)
// and from ReleaseService.CreateRelease, so every create path enqueues without
// any per-site bookkeeping.
//
// Two properties must hold at once, and they are in tension inside a single
// Postgres transaction:
//
//   - ATOMIC with the create. The job must live or die with the entity: if the
//     surrounding create rolls back, the job must NOT exist (no orphan); if it
//     commits, the job exists. => the insert runs on the caller's tx.
//   - BEST-EFFORT. An enqueue failure must NEVER fail the entity create.
//
// The tension: in Postgres, ANY failed statement aborts the WHOLE transaction —
// a later COMMIT silently becomes a ROLLBACK. So merely swallowing the insert's
// error would NOT save the create; the create would still be lost at commit and
// the caller would see a phantom success (non-nil entity, nil error, but no row
// in the DB). The fix is a SAVEPOINT: the insert runs in a nested transaction
// (GORM emits SAVEPOINT / ROLLBACK TO SAVEPOINT when the receiver is already a
// tx), so a failed insert rolls back ONLY the savepoint and leaves the outer
// transaction healthy to commit the entity. When called on a non-tx *gorm.DB the
// nested call is a standalone tx — still isolating the failure.
//
// ON CONFLICT DO NOTHING makes a re-enqueue a no-op against the
// one-active-job-per-entity partial unique index (uq_image_enrich_queue_active),
// so the common "already queued" case is not even an error.
func enqueueImageEnrich(tx *gorm.DB, entityType string, entityID uint) {
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
