package auth

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

const (
	// AuthEmailOutboxLoopName is the poller's name in logs.
	AuthEmailOutboxLoopName = "auth_email_outbox"

	// DisableAuthEmailOutboxEnvVar set to "1" switches the outbox off at both
	// ends: Enqueue refuses new rows and the poller sends nothing, so no
	// magic-link or account-recovery email goes out while it is set. The
	// poller re-reads it every tick, so setting it on a running server stops
	// sending within one interval.
	DisableAuthEmailOutboxEnvVar = "DISABLE_AUTH_EMAIL_OUTBOX"

	// defaultAuthEmailOutboxInterval is short because a person is waiting on
	// the email. A tick with nothing due is one indexed read of the pending
	// rows.
	defaultAuthEmailOutboxInterval = 5 * time.Second

	// defaultAuthEmailOutboxBatch bounds one tick's wall-clock: each row is
	// one synchronous provider call, bounded by the email client's 15 s
	// timeout, so a tick is at most about 150 s.
	defaultAuthEmailOutboxBatch = 10

	// authEmailMaxAge is how long a request may wait before it is dropped
	// unsent. Someone who asked for a sign-in or recovery link that long ago
	// has given up or asked again, and a late email reads as unsolicited. It
	// also bounds what a backlog can send when the poller comes back from an
	// outage or the kill switch is cleared.
	authEmailMaxAge = 15 * time.Minute

	// authEmailRetryBase is the first retry delay after a failed send; each
	// further attempt doubles it. With the table's default of three attempts
	// a row is retried after 30 s and 60 s, well inside authEmailMaxAge.
	authEmailRetryBase = 30 * time.Second

	// authEmailStaleReclaim must exceed the worst-case tick wall-clock (about
	// 150 s, see defaultAuthEmailOutboxBatch), or a row still being sent is
	// reclaimed and sent twice.
	authEmailStaleReclaim = 10 * time.Minute

	// authEmailRetention is how long terminal rows stay as an audit trail of
	// what was sent, skipped or given up on.
	authEmailRetention = 7 * 24 * time.Hour

	// authEmailHousekeepingEvery spaces the reclaim, reap and prune passes,
	// which need not run on every short tick.
	authEmailHousekeepingEvery = time.Minute

	// authEmailEnqueueTimeout bounds the request path's insert.
	authEmailEnqueueTimeout = 5 * time.Second

	// authEmailFinalizeBudget caps the finalize write after a send.
	authEmailFinalizeBudget = 30 * time.Second
)

// Reasons recorded in last_error. They are fixed strings so an operator can
// count each outcome with one query.
const (
	authEmailSkipExpired     = "request expired before delivery"
	authEmailSkipAccountGone = "account no longer exists"
	authEmailSkipIneligible  = "account no longer eligible"
	authEmailFailUnknownKind = "unknown email kind"
	authEmailFailStranded    = "stranded in processing after max attempts"
)

// ErrAuthEmailOutboxDisabled is Enqueue's error while
// DisableAuthEmailOutboxEnvVar is set.
var ErrAuthEmailOutboxDisabled = errors.New("auth email outbox is switched off (" + DisableAuthEmailOutboxEnvVar + "=1)")

// AuthEmailOutboxDisabled reports whether the kill switch is set. Enqueue and
// the poller both read it through this function, so the two ends cannot
// disagree about its spelling or parse.
func AuthEmailOutboxDisabled() bool {
	return shared.EnvServiceDisabled(DisableAuthEmailOutboxEnvVar)
}

// The account reads, token mints and sends the poller needs. The container's
// user, JWT and email services satisfy them.
type (
	authEmailUsers interface {
		GetUserByID(userID uint) (*authm.User, error)
		IsAccountRecoverable(user *authm.User) bool
		GetDaysUntilPermanentDeletion(user *authm.User) int
	}
	authEmailTokens interface {
		CreateMagicLinkToken(userID uint, email string) (string, error)
		CreateAccountRecoveryToken(userID uint, email string) (string, error)
	}
	authEmailSender interface {
		IsConfigured() bool
		SendMagicLinkEmail(toEmail, token string) error
		SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error
	}
)

// AuthEmailOutbox queues and sends magic-link and account-recovery emails.
//
// The request handlers call Enqueue for an eligible account and answer at
// once; the poller sends. A handler therefore does the same work for a known
// and an unknown address apart from one indexed insert, and its response time
// no longer depends on the email provider.
//
// The poller reads the account and mints the token at send time:
//
//   - no bearer token is stored in the table;
//   - a link's lifetime starts when it is sent, so a retried send still
//     delivers a link with its whole life;
//   - the token carries the account's current address, which is the address
//     the link verifies against, and the email goes to that address;
//   - eligibility is checked again, with the rule the handler applied, so an
//     account restored, deleted or changed while queued gets no stale email.
//
// Claiming uses SELECT ... FOR UPDATE SKIP LOCKED, and rows flip to
// `processing` inside the claim's transaction; the send runs outside it.
// Finalize writes are guarded by status = 'processing' (shared.JobQueue). A
// reclaimed row that a slow worker then finalizes is sent twice; the stale
// reclaim window exceeds the worst-case tick to keep that from happening.
type AuthEmailOutbox struct {
	db     *gorm.DB
	users  authEmailUsers
	tokens authEmailTokens
	sender authEmailSender
	queue  *shared.JobQueue[authm.AuthEmailOutboxItem]

	interval time.Duration
	batch    int

	// lastHousekeeping is read and written only by the loop goroutine.
	lastHousekeeping time.Time

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

var _ contracts.AuthEmailOutboxInterface = (*AuthEmailOutbox)(nil)

// NewAuthEmailOutbox constructs the outbox over database.
func NewAuthEmailOutbox(database *gorm.DB, users authEmailUsers, tokens authEmailTokens, sender authEmailSender) *AuthEmailOutbox {
	return &AuthEmailOutbox{
		db:     database,
		users:  users,
		tokens: tokens,
		sender: sender,
		queue: shared.NewJobQueue[authm.AuthEmailOutboxItem](database, "auth email outbox",
			shared.QueueStatuses{
				Pending:    authm.AuthEmailStatusPending,
				Processing: authm.AuthEmailStatusProcessing,
				Failed:     authm.AuthEmailStatusFailed,
				Terminal:   []string{authm.AuthEmailStatusSent, authm.AuthEmailStatusSkipped, authm.AuthEmailStatusFailed},
			}, slog.Default()),
		interval: shared.EnvPositiveDuration("AUTH_EMAIL_OUTBOX_INTERVAL_SECONDS", time.Second, defaultAuthEmailOutboxInterval),
		batch:    shared.EnvPositiveInt("AUTH_EMAIL_OUTBOX_BATCH", defaultAuthEmailOutboxBatch),
		stopCh:   make(chan struct{}),
		logger:   slog.Default(),
	}
}

// Enqueue queues one email of kind for userID. A call while one is already
// queued for the same account and kind is a no-op (the partial unique index
// uq_auth_email_outbox_active with ON CONFLICT DO NOTHING).
//
// The insert runs on a context detached from the request's cancellation and
// bounded by authEmailEnqueueTimeout: a client that disconnects after asking
// still gets its email, and a stalled database cannot hold the request open.
func (o *AuthEmailOutbox) Enqueue(ctx context.Context, kind authm.AuthEmailKind, userID uint) error {
	if AuthEmailOutboxDisabled() {
		return ErrAuthEmailOutboxDisabled
	}
	ictx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authEmailEnqueueTimeout)
	defer cancel()
	item := authm.AuthEmailOutboxItem{
		Kind:   kind,
		UserID: userID,
		Status: authm.AuthEmailStatusPending,
	}
	return o.db.WithContext(ictx).Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error
}

// Start begins the poller. The first tick is at most one interval in.
func (o *AuthEmailOutbox) Start(ctx context.Context) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		shared.RunScheduledLoop(ctx, shared.LoopConfig{
			Name:     AuthEmailOutboxLoopName,
			Interval: o.interval,
			StopCh:   o.stopCh,
		}, o.processTick)
	}()
	o.logger.Info("auth email outbox poller started", "interval", o.interval, "batch", o.batch)
}

// Stop stops the poller and waits for an in-flight tick to return.
func (o *AuthEmailOutbox) Stop() {
	close(o.stopCh)
	o.wg.Wait()
	o.logger.Info("auth email outbox poller stopped")
}

// RunNow runs one tick immediately, housekeeping included.
func (o *AuthEmailOutbox) RunNow(ctx context.Context) {
	o.lastHousekeeping = time.Time{}
	o.processTick(ctx)
}

// processTick runs housekeeping when it is due, then claims and sends one
// batch. Rows are finalized one at a time, so a failed send never re-queues a
// neighbour that was already sent.
func (o *AuthEmailOutbox) processTick(ctx context.Context) {
	if AuthEmailOutboxDisabled() {
		return
	}
	// A process with no email provider leaves the rows for one that has one.
	// Claiming them here would spend every row's attempts on a send that
	// cannot succeed.
	if !o.sender.IsConfigured() {
		return
	}

	if time.Since(o.lastHousekeeping) >= authEmailHousekeepingEvery {
		o.housekeep(ctx)
		o.lastHousekeeping = time.Now()
	}

	items, err := o.claim(ctx)
	if err != nil {
		shared.LogCycleFailure(o.logger, "auth email outbox: claim failed", err)
		return
	}

	var sent, other int
	for i := range items {
		// A cancelled tick (a deploy) hands every row it has not reached back
		// to `pending` with the claim's attempt undone, so the next process
		// sends them on its first tick.
		if ctx.Err() != nil {
			o.requeueWithoutAttempt(ctx, items[i:])
			o.logger.Info("auth email outbox: tick canceled, requeued", "count", len(items[i:]))
			break
		}
		if o.processItem(ctx, items[i]) {
			sent++
		} else {
			other++
		}
	}
	if len(items) > 0 {
		o.logger.Info("auth email outbox tick", "claimed", len(items), "sent", sent, "other", other)
	}
}

// claim takes up to batch pending rows whose next_attempt_at has come and
// that have attempts left, flipped to `processing` with attempts+1. Returned
// rows carry their pre-increment attempts.
func (o *AuthEmailOutbox) claim(ctx context.Context) ([]authm.AuthEmailOutboxItem, error) {
	return o.queue.ClaimReady(ctx, o.batch, "next_attempt_at <= NOW()")
}

// processItem sends one row's email and records the outcome. It returns true
// when the email was sent.
func (o *AuthEmailOutbox) processItem(ctx context.Context, item authm.AuthEmailOutboxItem) bool {
	// Age first: it is the one bound that retires every row, whatever its
	// attempts.
	if time.Since(item.CreatedAt) > authEmailMaxAge {
		o.markSkipped(ctx, item, authEmailSkipExpired)
		return false
	}

	skipReason, err := o.deliver(item)
	switch {
	case errors.Is(err, errUnknownAuthEmailKind):
		o.markFailed(ctx, item, err)
		return false
	case err != nil:
		o.markFailedOrRetry(ctx, item, err)
		return false
	case skipReason != "":
		o.markSkipped(ctx, item, skipReason)
		return false
	}
	o.markSent(ctx, item)
	return true
}

var errUnknownAuthEmailKind = errors.New(authEmailFailUnknownKind)

// deliver reads the account, checks it still qualifies, mints the token and
// sends. It returns a skip reason when the account no longer qualifies, and
// an error when the attempt failed.
//
// The eligibility rules are the request handlers': a magic link needs a
// verified address; a recovery email needs a soft-deleted account still
// inside its recovery window.
func (o *AuthEmailOutbox) deliver(item authm.AuthEmailOutboxItem) (string, error) {
	user, err := o.users.GetUserByID(item.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return authEmailSkipAccountGone, nil
	}
	if err != nil {
		return "", err
	}
	if user == nil {
		return authEmailSkipAccountGone, nil
	}
	if user.Email == nil || *user.Email == "" {
		return authEmailSkipIneligible, nil
	}
	email := *user.Email

	switch item.Kind {
	case authm.AuthEmailKindMagicLink:
		if !user.EmailVerified {
			return authEmailSkipIneligible, nil
		}
		token, err := o.tokens.CreateMagicLinkToken(user.ID, email)
		if err != nil {
			return "", err
		}
		return "", o.sender.SendMagicLinkEmail(email, token)

	case authm.AuthEmailKindAccountRecovery:
		if user.IsActive || !o.users.IsAccountRecoverable(user) {
			return authEmailSkipIneligible, nil
		}
		daysRemaining := o.users.GetDaysUntilPermanentDeletion(user)
		token, err := o.tokens.CreateAccountRecoveryToken(user.ID, email)
		if err != nil {
			return "", err
		}
		return "", o.sender.SendAccountRecoveryEmail(email, token, daysRemaining)
	}
	return "", errUnknownAuthEmailKind
}

// housekeep reclaims rows stranded in `processing`, retires expired pending
// rows as a set, and prunes terminal rows past the retention window.
//
// Reaping expired rows before the claim keeps a backlog from occupying every
// batch with rows that would only be skipped.
func (o *AuthEmailOutbox) housekeep(ctx context.Context) {
	o.queue.ReclaimStale(ctx, authEmailStaleReclaim, authEmailFailStranded)

	res := o.db.WithContext(ctx).Model(&authm.AuthEmailOutboxItem{}).
		Where("status = ? AND created_at < ?", authm.AuthEmailStatusPending, time.Now().Add(-authEmailMaxAge)).
		Updates(map[string]interface{}{
			"status":     authm.AuthEmailStatusSkipped,
			"last_error": authEmailSkipExpired,
		})
	if res.Error != nil {
		shared.LogCycleFailure(o.logger, "auth email outbox: reap of expired rows failed", res.Error)
	} else if res.RowsAffected > 0 {
		o.logger.Warn("auth email outbox: dropped expired requests unsent",
			"count", res.RowsAffected, "max_age", authEmailMaxAge)
	}

	if n := o.queue.PruneTerminal(ctx, authEmailRetention); n > 0 {
		o.logger.Info("auth email outbox: pruned terminal rows", "count", n)
	}
}

func (o *AuthEmailOutbox) markSent(ctx context.Context, item authm.AuthEmailOutboxItem) {
	now := time.Now()
	o.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":     authm.AuthEmailStatusSent,
		"sent_at":    &now,
		"last_error": nil,
	})
	o.logger.Info("auth email outbox: sent", "id", item.ID, "kind", item.Kind, "user_id", item.UserID)
}

func (o *AuthEmailOutbox) markSkipped(ctx context.Context, item authm.AuthEmailOutboxItem, reason string) {
	o.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":     authm.AuthEmailStatusSkipped,
		"last_error": reason,
	})
	o.logger.Info("auth email outbox: skipped", "id", item.ID, "kind", item.Kind, "user_id", item.UserID, "reason", reason)
}

// markFailedOrRetry re-pends a row that has attempts left, with
// next_attempt_at pushed out by the backoff, and fails an exhausted one.
// item.Attempts is the pre-increment count, so this attempt is Attempts+1.
func (o *AuthEmailOutbox) markFailedOrRetry(ctx context.Context, item authm.AuthEmailOutboxItem, cause error) {
	if item.Attempts+1 >= item.MaxAttempts {
		o.markFailed(ctx, item, cause)
		return
	}
	delay := authEmailRetryBase << item.Attempts
	o.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":          authm.AuthEmailStatusPending,
		"last_error":      cause.Error(),
		"next_attempt_at": gorm.Expr("NOW() + make_interval(secs => ?)", delay.Seconds()),
	})
	o.logger.Warn("auth email outbox: send failed, will retry",
		"id", item.ID, "kind", item.Kind, "user_id", item.UserID, "retry_in", delay, "error", cause)
}

// markFailed records a terminal failure and reports it to Sentry. A provider
// error has already been reported once per attempt by the email service; this
// report is the one that says the person never got their email.
func (o *AuthEmailOutbox) markFailed(ctx context.Context, item authm.AuthEmailOutboxItem, cause error) {
	o.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":     authm.AuthEmailStatusFailed,
		"last_error": cause.Error(),
	})
	o.logger.Error("auth email outbox: giving up on email",
		"id", item.ID, "kind", item.Kind, "user_id", item.UserID, "error", cause)
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("service", AuthEmailOutboxLoopName)
		scope.SetTag("email_type", string(item.Kind))
		sentry.CaptureException(cause)
	})
}

// requeueWithoutAttempt returns rows to `pending` and undoes the claim's
// attempt, for rows a cancelled tick never reached.
func (o *AuthEmailOutbox) requeueWithoutAttempt(ctx context.Context, items []authm.AuthEmailOutboxItem) {
	o.finalize(ctx, shared.QueueRowIDs(items), map[string]interface{}{
		"status":   authm.AuthEmailStatusPending,
		"attempts": gorm.Expr("GREATEST(attempts - 1, 0)"),
	})
}

// finalize writes rows' outcome on a context detached from the tick's
// cancellation, so a shutdown right after a send still records it (a row left
// `processing` would be reclaimed and sent again), and bounded so a hung
// database cannot wedge Stop.
func (o *AuthEmailOutbox) finalize(ctx context.Context, ids []uint, updates map[string]interface{}) {
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authEmailFinalizeBudget)
	defer cancel()
	o.queue.Finalize(fctx, ids, updates)
}
