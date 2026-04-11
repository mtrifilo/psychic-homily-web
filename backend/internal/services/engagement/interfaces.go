package engagement

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for engagement services.
var (
	_ contracts.BookmarkServiceInterface      = (*BookmarkService)(nil)
	_ contracts.SavedShowServiceInterface     = (*SavedShowService)(nil)
	_ contracts.FavoriteVenueServiceInterface = (*FavoriteVenueService)(nil)
	_ contracts.CalendarServiceInterface      = (*CalendarService)(nil)
	_ contracts.ReminderServiceInterface      = (*ReminderService)(nil)
	_ contracts.AttendanceServiceInterface    = (*AttendanceService)(nil)
	_ contracts.FollowServiceInterface        = (*FollowService)(nil)
	_ contracts.CommentServiceInterface              = (*CommentService)(nil)
	_ contracts.CommentAdminServiceInterface         = (*CommentService)(nil)
	_ contracts.FieldNoteServiceInterface            = (*CommentService)(nil)
	_ contracts.CommentVoteServiceInterface          = (*CommentVoteService)(nil)
	_ contracts.CommentSubscriptionServiceInterface  = (*CommentSubscriptionService)(nil)
)
