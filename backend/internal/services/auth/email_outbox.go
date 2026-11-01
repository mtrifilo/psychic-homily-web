package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
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
	// magic-link, account-recovery or magic-link verification email goes out
	// while it is set. It is
	// read from the process environment, so it takes effect when a server
	// starts with it set (a restart or redeploy), not on a running one.
	DisableAuthEmailOutboxEnvVar = "DISABLE_AUTH_EMAIL_OUTBOX"

	// defaultAuthEmailOutboxInterval is short because a person is waiting on
	// the email. A tick with nothing due is one indexed read of the pending
	// rows.
	defaultAuthEmailOutboxInterval = 5 * time.Second

	// maxAuthEmailOutboxInterval caps AUTH_EMAIL_OUTBOX_INTERVAL_SECONDS far
	// below authEmailMaxAge, so a queued row is always claimed before it is
	// old enough to be reaped unsent.
	maxAuthEmailOutboxInterval = time.Minute

	// defaultAuthEmailOutboxBatch is how many rows one claim takes, and the
	// most AUTH_EMAIL_OUTBOX_BATCH may set. A tick claims again while claims
	// come back full, so the batch bounds one claim's lock, not the drain rate.
	defaultAuthEmailOutboxBatch = 10

	// authEmailSendWorkers is how many sends run at once. Resend allows 10
	// requests per second per team, shared with every other email this server
	// sends, so two concurrent sends (roughly 6 per second at typical
	// latency) leave headroom while draining a burst well inside
	// authEmailMaxAge.
	authEmailSendWorkers = 2

	// authEmailStartBudget is how long after a tick begins a worker may still
	// start a row, and the deadline of every account read the tick makes; a
	// row not started by then, or whose account read the deadline cut short,
	// goes back to `pending` for the next tick. Every row therefore has its
	// account read within this long of its claim and finishes within one
	// send (the email client's 15 s timeout) plus one finalize
	// (authEmailFinalizeBudget) after that, which keeps it well inside
	// authEmailStaleReclaim.
	authEmailStartBudget = 2 * time.Minute

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

	// authEmailStaleReclaim must exceed the longest a claimed row can stay in
	// flight, authEmailStartBudget plus one send and one finalize (about 2.5
	// minutes), or a row still being sent is reclaimed and sent twice. It is
	// also how long a row stranded in `processing` by a crash blocks that
	// account's next request of the same kind (the active-row unique index),
	// so it is kept close to that bound.
	authEmailStaleReclaim = 4 * time.Minute

	// authEmailRetention is how long terminal rows stay as an audit trail of
	// what was sent, skipped or given up on.
	authEmailRetention = 7 * 24 * time.Hour

	// authEmailHousekeepingEvery spaces the reclaim, reap and prune passes,
	// which need not run on every short tick.
	authEmailHousekeepingEvery = time.Minute

	// authEmailEnqueueTimeout bounds the request path's insert.
	authEmailEnqueueTimeout = 5 * time.Second

	// authEmailFinalizeBudget caps the finalize write after a send.
	authEmailFinalizeBudget = 10 * time.Second
)

// Reasons recorded in last_error. They are fixed strings so an operator can
// count each outcome with one query.
const (
	authEmailSkipExpired     = "request expired before delivery"
	authEmailSkipAccountGone = "account no longer exists"
	authEmailSkipIneligible  = "account no longer eligible"
	// authEmailSkipClosedAccount is an account that may no longer sign in
	// (authm.User.CanSignIn) queued for an email that goes only to one that
	// can (needsOpenAccount).
	authEmailSkipClosedAccount = "skipped_closed_account"
	authEmailFailUnknownKind   = "unknown email kind"
	authEmailFailStranded      = "stranded in processing after max attempts"
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

// The recovery rules, token mints and sends the poller needs. The container's
// user, JWT and email services satisfy them.
type (
	authEmailUsers interface {
		IsAccountRecoverable(user *authm.User) bool
		GetDaysUntilPermanentDeletion(user *authm.User) int
	}
	authEmailTokens interface {
		CreateMagicLinkToken(userID uint, email string) (string, error)
		CreateOAuthLinkToken(userID uint, email, pendingLink string) (string, error)
		CreateAccountRecoveryToken(userID uint, email string) (string, error)
		CreateVerificationToken(userID uint, email string) (string, error)
	}
	// authEmailSender is the shape of contracts.EmailServiceInterface's
	// sends, which every email service and its test doubles have.
	authEmailSender interface {
		IsConfigured() bool
		SendMagicLinkEmail(toEmail, token string) error
		SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error
		SendVerificationEmail(toEmail, token string) error
	}
	// authEmailContextSender is how the poller sends: each send takes the
	// worker's context, which carries the worker's Sentry hub, so a provider
	// error is reported there.
	authEmailContextSender interface {
		IsConfigured() bool
		SendMagicLinkEmailContext(ctx context.Context, toEmail, token string) error
		SendAccountRecoveryEmailContext(ctx context.Context, toEmail, token string, daysRemaining int) error
		SendVerificationEmailContext(ctx context.Context, toEmail, token string) error
	}
)

// contextFreeSender gives an authEmailSender without the context forms the
// poller's shape. Its sends ignore the context, so they report wherever the
// sender itself reports.
type contextFreeSender struct{ authEmailSender }

func (s contextFreeSender) SendMagicLinkEmailContext(_ context.Context, toEmail, token string) error {
	return s.SendMagicLinkEmail(toEmail, token)
}

func (s contextFreeSender) SendAccountRecoveryEmailContext(_ context.Context, toEmail, token string, daysRemaining int) error {
	return s.SendAccountRecoveryEmail(toEmail, token, daysRemaining)
}

func (s contextFreeSender) SendVerificationEmailContext(_ context.Context, toEmail, token string) error {
	return s.SendVerificationEmail(toEmail, token)
}

// asContextSender returns sender's context forms when it has them
// (notification.EmailService does), and sender adapted otherwise.
func asContextSender(sender authEmailSender) authEmailContextSender {
	if cs, ok := sender.(authEmailContextSender); ok {
		return cs
	}
	return contextFreeSender{sender}
}

// AuthEmailOutbox queues and sends magic-link, account-recovery, verification
// and pending-link emails.
//
// The enumeration-safe request handlers call Enqueue for a known account and
// answer at once; the poller sends. A handler's response time therefore does
// not depend on the email provider: a known address costs the account
// lookup's preloads and one indexed insert more than an unknown one.
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
// `processing` inside the claim's transaction; the sends run outside it, on
// authEmailSendWorkers workers. Finalize writes are guarded by status =
// 'processing' (shared.JobQueue). A reclaimed row that a slow worker then
// finalizes is sent twice; authEmailStartBudget keeps every row's time in
// flight under the stale reclaim window to keep that from happening.
type AuthEmailOutbox struct {
	db     *gorm.DB
	users  authEmailUsers
	tokens authEmailTokens
	sender authEmailContextSender
	queue  *shared.JobQueue[authm.AuthEmailOutboxItem]

	interval time.Duration
	batch    int
	workers  int

	// lastHousekeeping is read and written only by the loop goroutine.
	lastHousekeeping time.Time

	// sentryClient is the client the outbox's own hubs report through
	// (newReportHub).
	sentryClient func() *sentry.Client

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
		sender: asContextSender(sender),
		queue: shared.NewJobQueue[authm.AuthEmailOutboxItem](database, "auth email outbox",
			shared.QueueStatuses{
				Pending:    authm.AuthEmailStatusPending,
				Processing: authm.AuthEmailStatusProcessing,
				Failed:     authm.AuthEmailStatusFailed,
				Terminal:   []string{authm.AuthEmailStatusSent, authm.AuthEmailStatusSkipped, authm.AuthEmailStatusFailed},
			}, slog.Default()),
		interval: capped("AUTH_EMAIL_OUTBOX_INTERVAL_SECONDS",
			shared.EnvPositiveDuration("AUTH_EMAIL_OUTBOX_INTERVAL_SECONDS", time.Second, defaultAuthEmailOutboxInterval),
			maxAuthEmailOutboxInterval),
		batch: capped("AUTH_EMAIL_OUTBOX_BATCH",
			shared.EnvPositiveInt("AUTH_EMAIL_OUTBOX_BATCH", defaultAuthEmailOutboxBatch),
			defaultAuthEmailOutboxBatch),
		workers:      authEmailSendWorkers,
		sentryClient: func() *sentry.Client { return sentry.CurrentHub().Client() },
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
}

// newReportHub returns a Sentry hub of the outbox's own: the process's client
// over a new scope tagged with the service. It shares no scope with the
// global hub, so a report never carries a scope another goroutine pushed
// there.
func (o *AuthEmailOutbox) newReportHub() *sentry.Hub {
	scope := sentry.NewScope()
	scope.SetTag("service", AuthEmailOutboxLoopName)
	return sentry.NewHub(o.sentryClient(), scope)
}

// capped returns v, or limit with a warning when v exceeds it.
func capped[T int | time.Duration](key string, v, limit T) T {
	if v > limit {
		slog.Default().Warn("auth email outbox: env value above its limit, using the limit",
			"key", key, "value", v, "limit", limit)
		return limit
	}
	return v
}

// Enqueue queues one email of kind for userID. A call while one is already
// queued for the same account and kind is a no-op (the partial unique index
// uq_auth_email_outbox_active with ON CONFLICT DO NOTHING).
//
// The insert runs on a context detached from the request's cancellation and
// bounded by authEmailEnqueueTimeout: a client that disconnects after asking
// still gets its email, and a stalled database cannot hold the request open.
func (o *AuthEmailOutbox) Enqueue(ctx context.Context, kind authm.AuthEmailKind, userID uint) error {
	return o.enqueue(ctx, authm.AuthEmailOutboxItem{Kind: kind, UserID: userID})
}

// EnqueueOAuthLink queues the sign-in link that proves userID's mailbox for
// the pending link ref names, on Enqueue's terms. While one is queued for the
// account, a call for any pending link is a no-op.
func (o *AuthEmailOutbox) EnqueueOAuthLink(ctx context.Context, userID uint, ref string) error {
	return o.enqueue(ctx, authm.AuthEmailOutboxItem{Kind: authm.AuthEmailKindOAuthLink, UserID: userID, OAuthLinkRef: &ref})
}

func (o *AuthEmailOutbox) enqueue(ctx context.Context, item authm.AuthEmailOutboxItem) error {
	if AuthEmailOutboxDisabled() {
		return ErrAuthEmailOutboxDisabled
	}
	ictx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authEmailEnqueueTimeout)
	defer cancel()
	item.Status = authm.AuthEmailStatusPending
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

// processTick runs housekeeping when it is due, then claims and sends batches
// until a claim comes back short or authEmailStartBudget runs out. Rows are
// finalized one at a time, so a failed send never re-queues a neighbour that
// was already sent.
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

	startBy := time.Now().Add(authEmailStartBudget)
	var claimed int
	var c batchCounts
	for ctx.Err() == nil && time.Now().Before(startBy) {
		items, err := o.claim(ctx)
		if err != nil {
			shared.LogCycleFailure(o.logger, "auth email outbox: claim failed", err)
			break
		}
		claimed += len(items)
		o.sendBatch(ctx, items, startBy, &c)
		if len(items) < o.batch {
			break
		}
	}
	if n := c.expired.Load(); n > 0 {
		o.reportDroppedUnsent(int64(n))
	}
	if claimed > 0 {
		o.logger.Info("auth email outbox tick", "claimed", claimed,
			"sent", c.sent.Load(), "other", c.other.Load(), "requeued", c.requeued.Load(),
			"expired", c.expired.Load())
	}
}

// batchCounts tallies one tick's outcomes across the send workers.
type batchCounts struct {
	sent, other, requeued, expired atomic.Int32
}

// sendBatch processes items on o.workers workers. Each worker reports to
// Sentry on a hub of its own, carried on the context it passes down.
func (o *AuthEmailOutbox) sendBatch(ctx context.Context, items []authm.AuthEmailOutboxItem, startBy time.Time, c *batchCounts) {
	next := make(chan authm.AuthEmailOutboxItem)
	var wg sync.WaitGroup
	for w := 0; w < o.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wctx := sentry.SetHubOnContext(ctx, o.newReportHub())
			for item := range next {
				switch o.processItemContained(wctx, item, startBy) {
				case outcomeSent:
					c.sent.Add(1)
				case outcomeExpired:
					c.expired.Add(1)
				case outcomeRequeued:
					c.requeued.Add(1)
				default:
					c.other.Add(1)
				}
			}
		}()
	}
	for _, item := range items {
		next <- item
	}
	close(next)
	wg.Wait()
}

// processItemContained runs processItem and contains a panic to its row.
//
// The workers are goroutines of their own, outside RunScheduledLoop's
// per-cycle recover, so a panic that escaped here would end the process. It
// is recovered per row, not per worker, so a worker always returns to the
// channel and sendBatch's producer can never block on a worker that died.
// The row is marked failed rather than left `processing`, so one row that
// panics is retired once instead of being reclaimed into the same panic.
func (o *AuthEmailOutbox) processItemContained(ctx context.Context, item authm.AuthEmailOutboxItem, startBy time.Time) (outcome itemOutcome) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		outcome = outcomeOther
		o.logger.Error("auth email outbox: row panicked",
			"id", item.ID, "kind", item.Kind, "user_id", item.UserID,
			"panic", r, "stack", string(debug.Stack()))
		func() {
			defer func() {
				if r2 := recover(); r2 != nil {
					o.logger.Error("auth email outbox: marking a panicked row failed panicked too",
						"id", item.ID, "panic", r2)
				}
			}()
			o.markFailed(ctx, item, fmt.Errorf("panic while sending: %v", r))
		}()
	}()
	return o.processItem(ctx, item, startBy)
}

// itemOutcome is what processItem did with one row.
type itemOutcome int

const (
	outcomeOther itemOutcome = iota
	outcomeSent
	outcomeExpired
	outcomeRequeued
)

// claim takes up to batch pending rows whose next_attempt_at has come and
// that have attempts left, flipped to `processing` with attempts+1. Returned
// rows carry their pre-increment attempts.
func (o *AuthEmailOutbox) claim(ctx context.Context) ([]authm.AuthEmailOutboxItem, error) {
	return o.queue.ClaimReady(ctx, o.batch, "next_attempt_at <= NOW()")
}

// processItem sends one row's email and records the outcome.
//
// A row reached after the tick is cancelled (a deploy) or after startBy, and
// a row whose account read either of those cut short, goes back to `pending`
// with the claim's attempt undone, so the next tick sends it.
func (o *AuthEmailOutbox) processItem(ctx context.Context, item authm.AuthEmailOutboxItem, startBy time.Time) itemOutcome {
	if ctx.Err() != nil || !time.Now().Before(startBy) {
		return o.requeueWithoutAttempt(ctx, item)
	}

	// Age next: it is the one bound that retires every row, whatever its
	// attempts. The tick reports these to Sentry once, with the count.
	if time.Since(item.CreatedAt) > authEmailMaxAge {
		o.markSkipped(ctx, item, authEmailSkipExpired)
		return outcomeExpired
	}

	skipReason, err := o.deliver(ctx, item, startBy)
	switch {
	case errors.Is(err, errAccountReadCutShort):
		return o.requeueWithoutAttempt(ctx, item)
	case errors.Is(err, errUnknownAuthEmailKind):
		o.markFailed(ctx, item, err)
		return outcomeOther
	case err != nil:
		o.markFailedOrRetry(ctx, item, err)
		return outcomeOther
	case skipReason != "":
		o.markSkipped(ctx, item, skipReason)
		return outcomeOther
	}
	o.markSent(ctx, item)
	return outcomeSent
}

var (
	errUnknownAuthEmailKind = errors.New(authEmailFailUnknownKind)
	// errAccountReadCutShort is an account read that ended because the tick
	// was cancelled or its start budget ran out, not because the read failed.
	errAccountReadCutShort = errors.New("account read cut short by the end of the tick")
)

// deliver reads the account, checks it still qualifies, mints the token and
// sends. It returns a skip reason when the account no longer qualifies, and
// an error when the attempt failed.
//
// The eligibility rules: every kind but recovery needs an account that may
// sign in (needsOpenAccount); a magic link needs a verified address; a
// verification email needs an address not yet verified; a recovery email
// needs a soft-deleted account still inside its recovery window, the rule
// the recovery request and confirm handlers apply; a pending-link email
// needs the pending link it names.
func (o *AuthEmailOutbox) deliver(ctx context.Context, item authm.AuthEmailOutboxItem, startBy time.Time) (string, error) {
	user, err := o.readAccount(ctx, item.UserID, startBy)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return authEmailSkipAccountGone, nil
	}
	if err != nil {
		return "", err
	}
	if needsOpenAccount(item.Kind) && !user.CanSignIn() {
		return authEmailSkipClosedAccount, nil
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
		return "", o.sender.SendMagicLinkEmailContext(ctx, email, token)

	case authm.AuthEmailKindOAuthLink:
		// No verified-address rule: proving the mailbox is what this link is
		// for, and following it verifies the address. Whether the pending
		// link is still there is decided when the link is followed
		// (AuthService.CompleteOAuthLinkByEmail); the request kept it alive
		// for as long as this send may happen (HoldOAuthLinkForEmail).
		if item.OAuthLinkRef == nil {
			return authEmailSkipIneligible, nil
		}
		token, err := o.tokens.CreateOAuthLinkToken(user.ID, email, *item.OAuthLinkRef)
		if err != nil {
			return "", err
		}
		return "", o.sender.SendMagicLinkEmailContext(ctx, email, token)

	case authm.AuthEmailKindVerification:
		if user.EmailVerified {
			return authEmailSkipIneligible, nil
		}
		token, err := o.tokens.CreateVerificationToken(user.ID, email)
		if err != nil {
			return "", err
		}
		return "", o.sender.SendVerificationEmailContext(ctx, email, token)

	case authm.AuthEmailKindAccountRecovery:
		if user.IsActive || !o.users.IsAccountRecoverable(user) {
			return authEmailSkipIneligible, nil
		}
		daysRemaining := o.users.GetDaysUntilPermanentDeletion(user)
		token, err := o.tokens.CreateAccountRecoveryToken(user.ID, email)
		if err != nil {
			return "", err
		}
		return "", o.sender.SendAccountRecoveryEmailContext(ctx, email, token, daysRemaining)
	}
	return "", errUnknownAuthEmailKind
}

// needsOpenAccount reports whether kind's email goes only to an account that
// may sign in. Every kind does but recovery, which is how a closed account
// inside its recovery window is reopened, so a kind without a rule of its own
// gets the stricter one.
func needsOpenAccount(kind authm.AuthEmailKind) bool {
	return kind != authm.AuthEmailKindAccountRecovery
}

// readAccount reads the account a row names, closed or not, with startBy as
// the read's deadline. It reads the users table itself because the user
// service's reads take no context to carry that deadline. A read that the deadline or the tick's cancellation
// ended returns errAccountReadCutShort.
func (o *AuthEmailOutbox) readAccount(ctx context.Context, userID uint, startBy time.Time) (*authm.User, error) {
	rctx, cancel := context.WithDeadline(ctx, startBy)
	defer cancel()
	var user authm.User
	err := o.db.WithContext(rctx).Take(&user, userID).Error
	switch {
	case err == nil:
		return &user, nil
	case !errors.Is(err, gorm.ErrRecordNotFound) && rctx.Err() != nil:
		return nil, fmt.Errorf("%w: %w", errAccountReadCutShort, err)
	}
	return nil, err
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
		o.reportDroppedUnsent(res.RowsAffected)
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

// markFailed records a terminal failure and reports it to Sentry on the hub
// ctx carries (the worker's), or on a new outbox hub when it carries none. A
// provider error has already been reported once per attempt by the email
// service; this report is the one that says the person never got their
// email.
func (o *AuthEmailOutbox) markFailed(ctx context.Context, item authm.AuthEmailOutboxItem, cause error) {
	o.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":     authm.AuthEmailStatusFailed,
		"last_error": cause.Error(),
	})
	o.logger.Error("auth email outbox: giving up on email",
		"id", item.ID, "kind", item.Kind, "user_id", item.UserID, "error", cause)
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = o.newReportHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("email_type", string(item.Kind))
		hub.CaptureException(cause)
	})
}

// reportDroppedUnsent reports to Sentry that n requests aged out unsent: each
// is a person who asked for a sign-in, recovery or verification link and got
// none. One event per reap pass and one per tick, so a backlog is a few alerts
// rather than thousands.
func (o *AuthEmailOutbox) reportDroppedUnsent(n int64) {
	hub := o.newReportHub()
	hub.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelWarning)
		scope.SetExtra("count", n)
		scope.SetExtra("max_age", authEmailMaxAge.String())
	})
	hub.CaptureMessage("auth email outbox: requests expired unsent")
}

// requeueWithoutAttempt returns a row to `pending` and undoes the claim's
// attempt, for a row the tick did not get to send (processItem).
func (o *AuthEmailOutbox) requeueWithoutAttempt(ctx context.Context, item authm.AuthEmailOutboxItem) itemOutcome {
	o.finalize(ctx, []uint{item.ID}, map[string]interface{}{
		"status":   authm.AuthEmailStatusPending,
		"attempts": gorm.Expr("GREATEST(attempts - 1, 0)"),
	})
	return outcomeRequeued
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
