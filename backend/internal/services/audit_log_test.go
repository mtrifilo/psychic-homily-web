package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewAuditLogService(t *testing.T) {
	svc := NewAuditLogService(nil)
	assert.NotNil(t, svc)
}

func TestAuditLogService_NilDatabase(t *testing.T) {
	svc := &AuditLogService{db: nil}

	t.Run("LogAction", func(t *testing.T) {
		// LogAction is fire-and-forget — does not return an error, just logs
		// Should not panic with nil db
		assert.NotPanics(t, func() {
			svc.LogAction(1, "approve_show", "show", 1, nil)
		})
	})

	t.Run("GetAuditLogs", func(t *testing.T) {
		resp, total, err := svc.GetAuditLogs(10, 0, AuditLogFilters{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type AuditLogServiceIntegrationTestSuite struct {
	suite.Suite
	container       testcontainers.Container
	db              *gorm.DB
	auditLogService *AuditLogService
	ctx             context.Context
}

func (suite *AuditLogServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	// audit_logs needs: initial schema (users table for FK), and 000022 (audit_logs table)
	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000014_add_account_lockout.up.sql",
		"000022_add_audit_logs.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
		"000032_add_favorite_cities.up.sql",
	}
	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", m))
		if err != nil {
			suite.T().Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			suite.T().Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	suite.auditLogService = &AuditLogService{db: db}
}

func (suite *AuditLogServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *AuditLogServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestAuditLogServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AuditLogServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *AuditLogServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("admin-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Admin"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// =============================================================================
// Group 1: LogAction
// =============================================================================

func (suite *AuditLogServiceIntegrationTestSuite) TestLogAction_Success() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 42, nil)

	// Verify the log was created
	var log models.AuditLog
	err := suite.db.First(&log).Error
	suite.Require().NoError(err)
	suite.Equal(user.ID, *log.ActorID)
	suite.Equal("approve_show", log.Action)
	suite.Equal("show", log.EntityType)
	suite.Equal(uint(42), log.EntityID)
	suite.Nil(log.Metadata)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestLogAction_WithMetadata() {
	user := suite.createTestUser()

	metadata := map[string]interface{}{
		"old_status": "pending",
		"new_status": "approved",
		"show_title": "Great Show",
	}

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 42, metadata)

	var log models.AuditLog
	err := suite.db.First(&log).Error
	suite.Require().NoError(err)
	suite.Require().NotNil(log.Metadata)
	suite.Contains(string(*log.Metadata), "old_status")
	suite.Contains(string(*log.Metadata), "approved")
}

func (suite *AuditLogServiceIntegrationTestSuite) TestLogAction_NilMetadata() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "reject_show", "show", 10, nil)

	var log models.AuditLog
	err := suite.db.First(&log).Error
	suite.Require().NoError(err)
	suite.Nil(log.Metadata)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestLogAction_AllInstrumentedActions() {
	user := suite.createTestUser()

	actions := []struct {
		action     string
		entityType string
	}{
		{"approve_show", "show"},
		{"reject_show", "show"},
		{"verify_venue", "venue"},
		{"approve_venue_edit", "venue_edit"},
		{"reject_venue_edit", "venue_edit"},
		{"dismiss_report", "show_report"},
		{"resolve_report", "show_report"},
		{"resolve_report_with_flag", "show_report"},
	}

	for i, a := range actions {
		suite.auditLogService.LogAction(user.ID, a.action, a.entityType, uint(i+1), nil)
	}

	// Verify all 8 were logged
	var count int64
	suite.db.Model(&models.AuditLog{}).Count(&count)
	suite.Equal(int64(8), count)
}

// =============================================================================
// Group 2: GetAuditLogs
// =============================================================================

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_Success() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 1, nil)
	suite.auditLogService.LogAction(user.ID, "reject_show", "show", 2, nil)

	resp, total, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{})

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
	// Should be ordered by created_at DESC (most recent first)
	suite.Equal("reject_show", resp[0].Action)
	suite.Equal("approve_show", resp[1].Action)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_IncludesActorEmail() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 1, nil)

	resp, _, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(*user.Email, resp[0].ActorEmail)
	suite.Equal(user.ID, *resp[0].ActorID)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_IncludesMetadata() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 1, map[string]interface{}{
		"title": "Test Show",
	})

	resp, _, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().NotNil(resp[0].Metadata)
	suite.Equal("Test Show", resp[0].Metadata["title"])
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_FilterByEntityType() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 1, nil)
	suite.auditLogService.LogAction(user.ID, "verify_venue", "venue", 1, nil)

	resp, total, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{EntityType: "venue"})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("verify_venue", resp[0].Action)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_FilterByAction() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 1, nil)
	suite.auditLogService.LogAction(user.ID, "reject_show", "show", 2, nil)
	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 3, nil)

	resp, total, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{Action: "approve_show"})

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
	for _, r := range resp {
		suite.Equal("approve_show", r.Action)
	}
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_FilterByActorID() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	suite.auditLogService.LogAction(user1.ID, "approve_show", "show", 1, nil)
	suite.auditLogService.LogAction(user2.ID, "reject_show", "show", 2, nil)

	resp, total, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{ActorID: &user2.ID})

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("reject_show", resp[0].Action)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_CombinedFilters() {
	user := suite.createTestUser()

	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 1, nil)
	suite.auditLogService.LogAction(user.ID, "verify_venue", "venue", 1, nil)
	suite.auditLogService.LogAction(user.ID, "approve_show", "show", 2, nil)

	resp, total, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{
		EntityType: "show",
		Action:     "approve_show",
		ActorID:    &user.ID,
	})

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_Pagination() {
	user := suite.createTestUser()

	for i := 0; i < 5; i++ {
		suite.auditLogService.LogAction(user.ID, "approve_show", "show", uint(i+1), nil)
	}

	// Page 1
	resp1, total, err := suite.auditLogService.GetAuditLogs(2, 0, AuditLogFilters{})
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.auditLogService.GetAuditLogs(2, 2, AuditLogFilters{})
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// Page 3
	resp3, _, err := suite.auditLogService.GetAuditLogs(2, 4, AuditLogFilters{})
	suite.Require().NoError(err)
	suite.Len(resp3, 1)

	// No overlap
	suite.NotEqual(resp1[0].ID, resp2[0].ID)
}

func (suite *AuditLogServiceIntegrationTestSuite) TestGetAuditLogs_Empty() {
	resp, total, err := suite.auditLogService.GetAuditLogs(10, 0, AuditLogFilters{})

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}
