package admin

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for admin services.
var (
	_ contracts.AdminStatsServiceInterface   = (*AdminStatsService)(nil)
	_ contracts.AuditLogServiceInterface     = (*AuditLogService)(nil)
	_ contracts.DataSyncServiceInterface     = (*DataSyncService)(nil)
	_ contracts.ShowReportServiceInterface   = (*ShowReportService)(nil)
	_ contracts.ArtistReportServiceInterface = (*ArtistReportService)(nil)
	_ contracts.APITokenServiceInterface     = (*APITokenService)(nil)
	_ contracts.RevisionServiceInterface     = (*RevisionService)(nil)
	// CleanupService has no interface in contracts — it's a lifecycle service.
)
