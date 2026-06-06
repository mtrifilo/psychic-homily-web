package community

import (
	"fmt"

	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-997: admin-queue list query for entity_requests. Added as a SIBLING to
// PSY-869's entityrequest.go (whose ListPending is pending-only + entity_type-
// only) so the admin list endpoint can also filter by decision_state and
// source_context without modifying PSY-869's owned logic.
//
// Defaults to decision_state='pending' (the admin's primary view) when no
// State filter is given, matching ListPending's behavior and the index in the
// migration (idx_entity_requests_state_type). Requester is preloaded so the
// queue can show who filed each request; Payload is already on the row for the
// per-type preview.
func (s *EntityRequestService) ListRequests(filters *contracts.EntityRequestFilters) ([]communitym.EntityRequest, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if filters == nil {
		filters = &contracts.EntityRequestFilters{}
	}

	query := s.db.Model(&communitym.EntityRequest{})

	// State: default to pending (the queue's primary view). The handler has
	// already validated any non-empty value against the model enum.
	state := filters.State
	if state == "" {
		state = string(communitym.EntityRequestStatePending)
	}
	query = query.Where("decision_state = ?", state)

	if filters.EntityType != "" {
		query = query.Where("entity_type = ?", filters.EntityType)
	}
	if filters.SourceContext != "" {
		query = query.Where("source_context = ?", filters.SourceContext)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count entity requests: %w", err)
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	var reqs []communitym.EntityRequest
	if err := query.Preload("Requester").
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&reqs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list entity requests: %w", err)
	}
	return reqs, total, nil
}
