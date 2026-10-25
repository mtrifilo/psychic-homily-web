package auth

import "time"

// AuthEmailKind names which email an auth_email_outbox row asks for. The values
// are the table's kind CHECK constraint.
type AuthEmailKind string

const (
	AuthEmailKindMagicLink       AuthEmailKind = "magic_link"
	AuthEmailKindAccountRecovery AuthEmailKind = "account_recovery"
	// AuthEmailKindVerification is the email-verification link a magic-link
	// request sends instead when the account is not yet verified.
	AuthEmailKindVerification AuthEmailKind = "verification"
	// AuthEmailKindOAuthLink is the sign-in link that proves the mailbox for a
	// pending provider link; OAuthLinkRef names that pending link.
	AuthEmailKindOAuthLink AuthEmailKind = "oauth_link"
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

// AuthEmailOutboxItem is one magic-link, account-recovery, verification or
// pending-link email a request handler has promised to send. The row names
// the account and the kind, and for a pending-link email the pending link by
// OAuthLinkRef, a hash no browser can present. The poller reads the account
// and mints the link token at send time, so the table never holds a bearer
// token.
type AuthEmailOutboxItem struct {
	ID     uint          `gorm:"primaryKey"`
	Kind   AuthEmailKind `gorm:"column:kind;not null"`
	UserID uint          `gorm:"column:user_id;not null"`
	// OAuthLinkRef is the token_hash of the pending link an
	// AuthEmailKindOAuthLink row proves the mailbox for, and nil for every
	// other kind (auth_email_outbox_oauth_link_ref_check).
	OAuthLinkRef  *string    `gorm:"column:oauth_link_ref"`
	Status        string     `gorm:"column:status;not null;default:'pending'"`
	Attempts      int        `gorm:"column:attempts;not null;default:0"`
	MaxAttempts   int        `gorm:"column:max_attempts;not null;default:3"`
	NextAttemptAt time.Time  `gorm:"column:next_attempt_at;not null;default:now()"`
	LastError     *string    `gorm:"column:last_error"`
	CreatedAt     time.Time  `gorm:"not null"`
	UpdatedAt     time.Time  `gorm:"not null"`
	SentAt        *time.Time `gorm:"column:sent_at"`
}

func (AuthEmailOutboxItem) TableName() string { return "auth_email_outbox" }

// QueueRowID implements shared.QueueRow.
func (i AuthEmailOutboxItem) QueueRowID() uint { return i.ID }
