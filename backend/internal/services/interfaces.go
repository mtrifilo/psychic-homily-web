package services

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for services that live in the
// root services package. Sub-packages have their own interfaces.go.
//
// Catalog services:      internal/services/catalog/interfaces.go
// Engagement services:   internal/services/engagement/interfaces.go
// Pipeline services:     internal/services/pipeline/interfaces.go
// Auth services:         internal/services/auth/interfaces.go
// Notification services: internal/services/notification/interfaces.go
// User services:         internal/services/user/interfaces.go
// Admin services:        internal/services/admin/interfaces.go
var (
	_ contracts.CollectionServiceInterface = (*CollectionService)(nil)
	_ contracts.RequestServiceInterface    = (*RequestService)(nil)
)
