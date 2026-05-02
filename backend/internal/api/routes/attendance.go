package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

// setupAttendanceRoutes configures show attendance (going/interested) endpoints.
// Public endpoints use optional auth (counts always available; user status if authenticated).
// Set/remove attendance requires authentication.
func setupAttendanceRoutes(rc RouteContext) {
	attendanceHandler := engagementh.NewAttendanceHandler(rc.SC.Attendance)

	// Public endpoints with optional auth (counts + user status if authenticated)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}/attendance", attendanceHandler.GetAttendanceHandler)
	huma.Post(optionalAuthGroup, "/shows/attendance/batch", attendanceHandler.BatchAttendanceHandler)

	// Protected endpoints (require authentication)
	huma.Post(rc.Protected, "/shows/{show_id}/attendance", attendanceHandler.SetAttendanceHandler)
	huma.Delete(rc.Protected, "/shows/{show_id}/attendance", attendanceHandler.RemoveAttendanceHandler)
	huma.Get(rc.Protected, "/attendance/my-shows", attendanceHandler.GetMyShowsHandler)
}
