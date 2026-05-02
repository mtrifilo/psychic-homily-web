package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

// setupCommentRoutes configures comment endpoints.
// Public routes use optional auth (could be used for user vote context in future).
// Protected routes require authentication.
// Admin routes require admin privileges.
func setupCommentRoutes(rc RouteContext) {
	commentHandler := engagementh.NewCommentHandler(rc.SC.Comment, rc.SC.Comment, rc.SC.CommentVote, rc.SC.AuditLog)
	commentAdminHandler := engagementh.NewCommentAdminHandler(rc.SC.Comment, rc.SC.AuditLog)

	// Public: list comments, get comment, get thread
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/entities/{entity_type}/{entity_id}/comments", commentHandler.ListCommentsHandler)
	huma.Get(optionalAuthGroup, "/comments/{comment_id}", commentHandler.GetCommentHandler)
	huma.Get(optionalAuthGroup, "/comments/{comment_id}/thread", commentHandler.GetThreadHandler)

	// Protected: create, reply, update, delete
	huma.Post(rc.Protected, "/entities/{entity_type}/{entity_id}/comments", commentHandler.CreateCommentHandler)
	huma.Post(rc.Protected, "/comments/{comment_id}/replies", commentHandler.CreateReplyHandler)
	huma.Put(rc.Protected, "/comments/{comment_id}", commentHandler.UpdateCommentHandler)
	huma.Delete(rc.Protected, "/comments/{comment_id}", commentHandler.DeleteCommentHandler)
	// PSY-296: owner-only reply-permission toggle.
	huma.Put(rc.Protected, "/comments/{comment_id}/reply-permission", commentHandler.UpdateReplyPermissionHandler)

	// Admin: comment moderation (PSY-423: rc.Admin enforces auth + IsAdmin)
	// NOTE: literal paths MUST be registered before parameterized paths to avoid
	// {comment_id} consuming "pending" as a value and returning 404.
	huma.Get(rc.Admin, "/admin/comments/pending", commentAdminHandler.AdminListPendingCommentsHandler)
	huma.Post(rc.Admin, "/admin/comments/{comment_id}/hide", commentAdminHandler.AdminHideCommentHandler)
	huma.Post(rc.Admin, "/admin/comments/{comment_id}/restore", commentAdminHandler.AdminRestoreCommentHandler)
	huma.Post(rc.Admin, "/admin/comments/{comment_id}/approve", commentAdminHandler.AdminApproveCommentHandler)
	huma.Post(rc.Admin, "/admin/comments/{comment_id}/reject", commentAdminHandler.AdminRejectCommentHandler)
	// Admin: edit history viewer (PSY-297)
	huma.Get(rc.Admin, "/admin/comments/{comment_id}/edits", commentAdminHandler.AdminGetCommentEditHistoryHandler)
}

// setupCommentVoteRoutes configures comment voting endpoints.
func setupCommentVoteRoutes(rc RouteContext) {
	commentVoteHandler := engagementh.NewCommentVoteHandler(rc.SC.CommentVote)

	// Protected: vote and unvote on comments
	huma.Post(rc.Protected, "/comments/{comment_id}/vote", commentVoteHandler.VoteCommentHandler)
	huma.Delete(rc.Protected, "/comments/{comment_id}/vote", commentVoteHandler.UnvoteCommentHandler)
}

// setupCommentSubscriptionRoutes configures comment subscription and unread tracking endpoints.
func setupCommentSubscriptionRoutes(rc RouteContext) {
	subHandler := engagementh.NewCommentSubscriptionHandler(rc.SC.CommentSubscription, rc.SC.AuditLog)

	// Protected: subscribe, unsubscribe, check status, mark read
	huma.Post(rc.Protected, "/entities/{entity_type}/{entity_id}/subscribe", subHandler.SubscribeHandler)
	huma.Delete(rc.Protected, "/entities/{entity_type}/{entity_id}/subscribe", subHandler.UnsubscribeHandler)
	huma.Get(rc.Protected, "/entities/{entity_type}/{entity_id}/subscribe/status", subHandler.SubscriptionStatusHandler)
	huma.Post(rc.Protected, "/entities/{entity_type}/{entity_id}/mark-read", subHandler.MarkReadHandler)
}

// setupFieldNoteRoutes configures field note endpoints on shows.
func setupFieldNoteRoutes(rc RouteContext) {
	fieldNoteHandler := engagementh.NewFieldNoteHandler(rc.SC.Comment, rc.SC.Comment, rc.SC.AuditLog)

	// Public: list field notes for a show
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}/field-notes", fieldNoteHandler.ListFieldNotesHandler)

	// Protected: create field note
	huma.Post(rc.Protected, "/shows/{show_id}/field-notes", fieldNoteHandler.CreateFieldNoteHandler)
}
