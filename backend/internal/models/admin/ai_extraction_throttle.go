package admin

import "time"

// AIExtractionThrottle is the per-user rolling-window counter backing the rate
// limit on the AI extraction routes (extract-show + extract-collection, PSY-855).
//
// One row per user. The window is a fixed 1-hour span starting at WindowStart;
// RequestCount accrues against it. The throttle service resets the window
// in-place (WindowStart=NOW, RequestCount=1) via an atomic upsert once the
// window has elapsed, so the table never grows beyond one row per user who has
// ever extracted.
//
// Postgres-backed (not in-memory) so the count is consistent across Vercel
// serverless instances — the Next.js BFF extract routes call the backend to
// increment-and-check before doing any (paid) Anthropic work.
type AIExtractionThrottle struct {
	UserID       uint      `json:"user_id" gorm:"column:user_id;primaryKey"`
	WindowStart  time.Time `json:"window_start" gorm:"column:window_start;not null"`
	RequestCount int       `json:"request_count" gorm:"column:request_count;not null;default:0"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AIExtractionThrottle) TableName() string { return "ai_extraction_throttle" }
