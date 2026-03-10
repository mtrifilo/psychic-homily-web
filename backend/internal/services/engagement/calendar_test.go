package engagement

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewCalendarService(t *testing.T) {
	svc := NewCalendarService(nil, nil)
	assert.NotNil(t, svc)
}

func TestCalendarService_NilDatabase(t *testing.T) {
	svc := &CalendarService{db: nil}

	t.Run("CreateToken", func(t *testing.T) {
		resp, err := svc.CreateToken(1, "http://localhost:8080")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetTokenStatus", func(t *testing.T) {
		resp, err := svc.GetTokenStatus(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("DeleteToken", func(t *testing.T) {
		err := svc.DeleteToken(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("ValidateCalendarToken", func(t *testing.T) {
		user, err := svc.ValidateCalendarToken("phcal_abc123")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("GenerateICSFeed", func(t *testing.T) {
		data, err := svc.GenerateICSFeed(1, "http://localhost:3000")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, data)
	})
}

func TestGenerateCalendarToken_Format(t *testing.T) {
	token, err := generateCalendarToken()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, CalendarTokenPrefix), "token should start with %s", CalendarTokenPrefix)
	// prefix "phcal_" (6) + hex-encoded 32 bytes (64) = 70 chars
	assert.Len(t, token, 6+64, "token should be prefix(6) + hex(64) = 70 chars")
}

func TestGenerateCalendarToken_Unique(t *testing.T) {
	token1, err1 := generateCalendarToken()
	token2, err2 := generateCalendarToken()
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, token1, token2, "two generated tokens should be different")
}

func TestGenerateICSFeed_Format(t *testing.T) {
	// Create a service with a mock saved show service
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        1,
				Slug:      "test-show",
				Title:     "Test Show",
				EventDate: time.Now().Add(24 * time.Hour),
				City:      ptrString("Phoenix"),
				State:     ptrString("AZ"),
				Status:    "approved",
				Venues: []contracts.VenueResponse{
					{ID: 1, Name: "The Venue", City: "Phoenix", State: "AZ"},
				},
				Artists: []contracts.ArtistResponse{
					{ID: 1, Name: "Artist One"},
					{ID: 2, Name: "Artist Two"},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotNil(t, data)

	icsStr := string(data)
	assert.Contains(t, icsStr, "BEGIN:VCALENDAR")
	assert.Contains(t, icsStr, "END:VCALENDAR")
	assert.Contains(t, icsStr, "BEGIN:VEVENT")
	assert.Contains(t, icsStr, "END:VEVENT")
	assert.Contains(t, icsStr, "SUMMARY:Test Show")
	assert.Contains(t, icsStr, "show-1@psychichomily.com")
	// ICS escapes commas per RFC 5545
	assert.Contains(t, icsStr, "The Venue\\, Phoenix\\, AZ")
	assert.Contains(t, icsStr, "Artist One")
	// ICS uses line folding (CRLF + space) for long lines, so unfold before checking
	unfolded := strings.ReplaceAll(icsStr, "\n ", "")
	assert.Contains(t, unfolded, "Artist Two")
	assert.Contains(t, icsStr, "https://psychichomily.com/shows/test-show")
	assert.Contains(t, icsStr, "METHOD:PUBLISH")
}

func TestGenerateICSFeed_SoldOutLabel(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        2,
				Slug:      "sold-out-show",
				Title:     "Hot Show",
				EventDate: time.Now().Add(48 * time.Hour),
				Status:    "approved",
				IsSoldOut: true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "SUMMARY:Hot Show [SOLD OUT]")
}

func TestGenerateICSFeed_FiltersCancelled(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:          3,
				Title:       "Cancelled Show",
				EventDate:   time.Now().Add(24 * time.Hour),
				Status:      "approved",
				IsCancelled: true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "Cancelled Show")
	assert.NotContains(t, string(data), "BEGIN:VEVENT")
}

func TestGenerateICSFeed_FiltersNonApproved(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        4,
				Title:     "Pending Show",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "pending",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "Pending Show")
}

func TestGenerateICSFeed_FiltersOldShows(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        5,
				Title:     "Old Show",
				EventDate: time.Now().AddDate(0, 0, -60), // 60 days ago
				Status:    "approved",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.NotContains(t, string(data), "Old Show")
}

func TestGenerateICSFeed_EmptyList(t *testing.T) {
	mockSvc := &mockSavedShowSvc{shows: []*contracts.SavedShowResponse{}, total: 0}
	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "BEGIN:VCALENDAR")
	assert.NotContains(t, string(data), "BEGIN:VEVENT")
}

func TestGenerateICSFeed_FallbackToID(t *testing.T) {
	mockShows := []*contracts.SavedShowResponse{
		{
			ShowResponse: contracts.ShowResponse{
				ID:        42,
				Slug:      "", // no slug
				Title:     "No Slug Show",
				EventDate: time.Now().Add(24 * time.Hour),
				Status:    "approved",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
	mockSvc := &mockSavedShowSvc{shows: mockShows, total: 1}

	svc := &CalendarService{db: &gorm.DB{}, savedShowSvc: mockSvc}
	data, err := svc.GenerateICSFeed(1, "https://psychichomily.com")
	assert.NoError(t, err)
	assert.Contains(t, string(data), "https://psychichomily.com/shows/42")
}

// =============================================================================
// Mock saved show service for unit tests
// =============================================================================

type mockSavedShowSvc struct {
	shows []*contracts.SavedShowResponse
	total int64
	err   error
}

func (m *mockSavedShowSvc) SaveShow(_, _ uint) error { return nil }
func (m *mockSavedShowSvc) UnsaveShow(_, _ uint) error { return nil }
func (m *mockSavedShowSvc) GetUserSavedShows(_ uint, _, _ int) ([]*contracts.SavedShowResponse, int64, error) {
	return m.shows, m.total, m.err
}
func (m *mockSavedShowSvc) IsShowSaved(_, _ uint) (bool, error) { return false, nil }
func (m *mockSavedShowSvc) GetSavedShowIDs(_ uint, _ []uint) (map[uint]bool, error) {
	return nil, nil
}

func ptrString(s string) *string { return &s }

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type CalendarIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	svc       *CalendarService
	ctx       context.Context
}

func (suite *CalendarIntegrationTestSuite) SetupSuite() {
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

	testutil.RunAllMigrations(suite.T(), sqlDB, filepath.Join("..", "..", "..", "db", "migrations"))

	mockSavedShows := &mockSavedShowSvc{shows: []*contracts.SavedShowResponse{}, total: 0}
	suite.svc = &CalendarService{db: db, savedShowSvc: mockSavedShows}
}

func (suite *CalendarIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *CalendarIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM calendar_tokens")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCalendarIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CalendarIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CalendarIntegrationTestSuite) createTestUser(active bool) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("cal-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Calendar"),
		LastName:      stringPtr("User"),
		IsActive:      active,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// =============================================================================
// CreateToken tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestCreateToken_Success() {
	user := suite.createTestUser(true)
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.True(strings.HasPrefix(resp.Token, CalendarTokenPrefix))
	suite.Contains(resp.FeedURL, "http://localhost:8080/calendar/phcal_")
	suite.NotZero(resp.CreatedAt)
}

func (suite *CalendarIntegrationTestSuite) TestCreateToken_ReplacesExisting() {
	user := suite.createTestUser(true)

	resp1, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	resp2, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	// New token should be different
	suite.NotEqual(resp1.Token, resp2.Token)

	// Old token should no longer validate
	_, err = suite.svc.ValidateCalendarToken(resp1.Token)
	suite.Error(err)
	suite.Contains(err.Error(), "invalid calendar token")

	// New token should validate
	validatedUser, err := suite.svc.ValidateCalendarToken(resp2.Token)
	suite.Require().NoError(err)
	suite.Equal(user.ID, validatedUser.ID)
}

// =============================================================================
// GetTokenStatus tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_NoToken() {
	user := suite.createTestUser(true)
	status, err := suite.svc.GetTokenStatus(user.ID)
	suite.Require().NoError(err)
	suite.False(status.HasToken)
	suite.Nil(status.CreatedAt)
}

func (suite *CalendarIntegrationTestSuite) TestGetTokenStatus_HasToken() {
	user := suite.createTestUser(true)
	_, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	status, err := suite.svc.GetTokenStatus(user.ID)
	suite.Require().NoError(err)
	suite.True(status.HasToken)
	suite.NotNil(status.CreatedAt)
}

// =============================================================================
// DeleteToken tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestDeleteToken_Success() {
	user := suite.createTestUser(true)
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	err = suite.svc.DeleteToken(user.ID)
	suite.NoError(err)

	// Token should no longer validate
	_, err = suite.svc.ValidateCalendarToken(resp.Token)
	suite.Error(err)

	// Status should show no token
	status, err := suite.svc.GetTokenStatus(user.ID)
	suite.Require().NoError(err)
	suite.False(status.HasToken)
}

func (suite *CalendarIntegrationTestSuite) TestDeleteToken_NotFound() {
	user := suite.createTestUser(true)
	err := suite.svc.DeleteToken(user.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "no calendar token found")
}

// =============================================================================
// ValidateCalendarToken tests
// =============================================================================

func (suite *CalendarIntegrationTestSuite) TestValidateCalendarToken_Success() {
	user := suite.createTestUser(true)
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	validatedUser, err := suite.svc.ValidateCalendarToken(resp.Token)
	suite.Require().NoError(err)
	suite.Equal(user.ID, validatedUser.ID)
}

func (suite *CalendarIntegrationTestSuite) TestValidateCalendarToken_Invalid() {
	_, err := suite.svc.ValidateCalendarToken("phcal_nonexistent_token")
	suite.Error(err)
	suite.Contains(err.Error(), "invalid calendar token")
}

func (suite *CalendarIntegrationTestSuite) TestValidateCalendarToken_InactiveUser() {
	user := suite.createTestUser(true) // create as active first
	resp, err := suite.svc.CreateToken(user.ID, "http://localhost:8080")
	suite.Require().NoError(err)

	// Deactivate the user
	suite.db.Model(&models.User{}).Where("id = ?", user.ID).Update("is_active", false)

	_, err = suite.svc.ValidateCalendarToken(resp.Token)
	suite.Error(err)
	suite.Contains(err.Error(), "user account is not active")
}
