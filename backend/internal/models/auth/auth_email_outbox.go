package auth

import "time"

// AuthEmailKind names which email an auth_email_outbox row asks for. The values
// are the table's kind CHECK constraint.
type AuthEmailKind string

const (
	AuthEmailKindMagicLink       AuthEmailKind = "magic_link"
	AuthEmailKindAccountRecovery AuthEmailKind = "account_recovery"
)

// auth_email_outbox status values.
//
//	pending -> processing (claimed) -> sent
//	                                -> skipped (the account is no longer eligible, or the request expired)
//	                                -> pending (retry after backoff) -> failed once attempts run out
const (
	AuthEmailStatusPending    = "pending"
	AuthEmailStatusProcessing = "processing"
	AuthEmailStatusSent       = "sent"
	AuthEmailStatusSkipped    = "skipped"
	AuthEmailStatusFailed     = "failed"
)

// AuthEmailOutboxItem is one magic-link or account-recovery email a request
// handler has promised to send. The row names the account and the kind only:
// the poller reads the account and mints the link token at send time, so the
// table never holds a bearer token.
type AuthEmailOutboxItem struct {
	ID            uint          `gorm:"primaryKey"`
	Kind          AuthEmailKind `gorm:"column:kind;not null"`
	UserID        uint          `gorm:"column:user_id;not null"`
	Status        string        `gorm:"column:status;not null;default:'pending'"`
	Attempts      int           `gorm:"column:attempts;not null;default:0"`
	MaxAttempts   int           `gorm:"column:max_attempts;not null;default:3"`
	NextAttemptAt time.Time     `gorm:"column:next_attempt_at;not null;default:now()"`
	LastError     *string       `gorm:"column:last_error"`
	CreatedAt     time.Time     `gorm:"not null"`
	UpdatedAt     time.Time     `gorm:"not null"`
	SentAt        *time.Time    `gorm:"column:sent_at"`
}

func (AuthEmailOutboxItem) TableName() string { return "auth_email_outbox" }

// QueueRowID implements shared.QueueRow.
func (i AuthEmailOutboxItem) QueueRowID() uint { return i.ID }
