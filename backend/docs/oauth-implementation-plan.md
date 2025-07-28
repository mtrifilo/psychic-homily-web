# OAuth2 Implementation Plan with Goth & JWT

## Overview

This document outlines the plan for implementing user authentication using Goth for OAuth2 providers (Google, GitHub, Instagram) with JWT-based authentication.

## 🎯 Goals

- **OAuth2 Authentication**: Google, GitHub, and Instagram login
- **JWT Authentication**: Stateless token-based authentication
- **User Management**: Profile management and preferences
- **Authorization**: Route protection and role-based access

## 📋 Implementation Phases

### Phase 1: Database & Models ✅ (Complete)

- [x] **User Tables Migration**: `000001_create_initial_schema.up.sql`
- [x] **Go Models**: `internal/models/user.go`
- [x] **Configuration**: Updated `internal/config/config.go`

### Phase 2: Goth Integration ✅ (Complete)

#### 2.1 Install Dependencies ✅

```bash
go get github.com/markbates/goth
go get github.com/markbates/goth/providers/google
go get github.com/markbates/goth/providers/github
go get github.com/markbates/goth/providers/instagram
go get github.com/gorilla/sessions
go get github.com/golang-jwt/jwt/v5
```

#### 2.2 Goth Setup ✅

- [x] Create `internal/auth/goth.go` for Goth configuration
- [x] Configure Google, GitHub, and Instagram providers
- [x] Set up temporary session store for OAuth flow

#### 2.3 Authentication Service ✅

- [x] Create `internal/services/auth.go` for business logic
- [x] Create `internal/services/jwt.go` for JWT handling
- [x] User creation/linking logic
- [x] JWT token generation and validation

### Phase 3: Authentication Handlers ✅ (Complete)

#### 3.1 OAuth Handlers ✅

- [x] `internal/api/handlers/auth.go`
  - [x] `OAuthLoginHandler` - Initiate OAuth flow
  - [x] `OAuthCallbackHandler` - Handle OAuth callback
  - [x] `LogoutHandler` - Logout user (JWT token invalidation)

#### 3.2 JWT Handlers ✅

- [x] `internal/api/handlers/auth.go` (continued)
  - [x] `RefreshTokenHandler` - Refresh JWT tokens
  - [x] `GetProfileHandler` - Get user profile

#### 3.3 User Management Handlers

- `internal/api/handlers/users.go`
  - `GetProfileHandler` - Get user profile
  - `UpdateProfileHandler` - Update profile
  - `GetPreferencesHandler` - Get preferences
  - `UpdatePreferencesHandler` - Update preferences

### Phase 4: Middleware & Security ✅ (Complete)

#### 4.1 JWT Authentication Middleware ✅

- [x] `internal/api/middleware/jwt.go`
  - [x] `JWTMiddleware` - Validate JWT tokens
  - [x] `GetUserFromContext` - Extract user from request context

#### 4.2 Security Features

- [x] JWT token validation
- [x] Token expiration handling
- [x] Secure token storage (client-side)

### Phase 5: Database Integration ✅ (Complete)

#### 5.1 Database Service ✅

- [x] `internal/database/connection.go` - Database connection
- [x] GORM integration with JWT authentication

#### 5.2 Repository Pattern

- `internal/repository/user.go` - User data access
- `internal/repository/oauth.go` - OAuth data access

## 🔧 Technical Implementation

### Goth Configuration ✅

```go
// internal/auth/goth.go
package auth

import (
    "github.com/markbates/goth"
    "github.com/markbates/goth/providers/google"
    "github.com/markbates/goth/providers/github"
    "github.com/markbates/goth/providers/instagram"
)

func SetupGoth(cfg *config.Config) {
    goth.UseProviders(
        google.New(cfg.OAuth.GoogleClientID, cfg.OAuth.GoogleClientSecret, cfg.OAuth.RedirectURL),
        github.New(cfg.OAuth.GitHubClientID, cfg.OAuth.GitHubClientSecret, cfg.OAuth.RedirectURL),
        instagram.New(cfg.OAuth.InstagramClientID, cfg.OAuth.InstagramClientSecret, cfg.OAuth.RedirectURL),
    )
}
```

### JWT Service ✅

```go
// internal/services/jwt.go
type JWTService struct {
    config *config.Config
}

func (s *JWTService) CreateToken(user *models.User) (string, error)
func (s *JWTService) ValidateToken(tokenString string) (*models.User, error)
func (s *JWTService) RefreshToken(tokenString string) (string, error)
```

### Authentication Flow ✅

1. **OAuth Login**:

   - User clicks "Login with Google/GitHub/Instagram"
   - Redirect to provider OAuth page
   - Provider redirects back with code
   - Exchange code for access token
   - Get user info from provider
   - Create/link user account
   - Generate JWT token
   - Return JWT token to frontend

2. **JWT Authentication**:
   - Frontend stores JWT token
   - Include JWT in Authorization header
   - Backend validates JWT on protected routes
   - Token refresh when needed

### JWT Management ✅

- **Token Generation**: Secure JWT with user claims
- **Token Validation**: Middleware validates tokens
- **Token Refresh**: Automatic token renewal
- **Token Expiry**: Configurable expiration times

## 🛡️ Security Considerations

### OAuth Security ✅

- **State Parameter**: CSRF protection
- **Secure Redirect**: HTTPS only in production
- **Token Storage**: Secure token storage
- **Provider Validation**: Validate provider responses

### JWT Security ✅

- **Secure Tokens**: Cryptographically signed JWTs
- **Token Expiry**: Short-lived access tokens
- **Refresh Tokens**: Long-lived refresh tokens
- **Token Storage**: Secure client-side storage

### Password Security

- **Bcrypt Hashing**: Secure password hashing
- **Password Policy**: Strong password requirements
- **Rate Limiting**: Prevent brute force attacks

## 📊 Database Schema ✅

### Users Table ✅

- `id`: Primary key
- `email`: Unique email (nullable for OAuth-only users)
- `username`: Unique username (nullable for OAuth-only users)
- `password_hash`: Bcrypt hash (nullable for OAuth-only users)
- `first_name`, `last_name`: User names
- `avatar_url`: Profile picture
- `bio`: User bio
- `is_active`: Account status
- `is_admin`: Admin privileges
- `email_verified`: Email verification status

### OAuth Accounts Table ✅

- `id`: Primary key
- `user_id`: Foreign key to users
- `provider`: OAuth provider (google, github, instagram)
- `provider_user_id`: External provider user ID
- `provider_email`: Email from provider
- `provider_name`: Name from provider
- `provider_avatar_url`: Avatar from provider
- `access_token`: OAuth access token (encrypted)
- `refresh_token`: OAuth refresh token (encrypted)
- `expires_at`: Token expiration

### User Preferences Table ✅

- `id`: Primary key
- `user_id`: Foreign key to users
- `notification_email`: Email notification preference
- `notification_push`: Push notification preference
- `theme`: UI theme preference
- `timezone`: User timezone
- `language`: User language

## 🚀 Next Steps

1. ✅ **Install Goth Dependencies**
2. ✅ **Create Goth Configuration**
3. ✅ **Implement JWT Service**
4. ✅ **Create OAuth Handlers**
5. ✅ **Add JWT Authentication Middleware**
6. ✅ **Test OAuth Flow**
7. [ ] **Add Local Authentication**
8. [ ] **Implement User Management**
9. [ ] **Add Security Features**
10. [ ] **Production Deployment**

## 📝 Environment Variables ✅

```bash
# OAuth Configuration
GOOGLE_CLIENT_ID=your_google_client_id
GOOGLE_CLIENT_SECRET=your_google_client_secret
GITHUB_CLIENT_ID=your_github_client_id
GITHUB_CLIENT_SECRET=your_github_client_secret
INSTAGRAM_CLIENT_ID=your_instagram_client_id
INSTAGRAM_CLIENT_SECRET=your_instagram_client_secret
OAUTH_REDIRECT_URL=http://localhost:8080/auth/callback
OAUTH_SECRET_KEY=your-secret-key-for-oauth-sessions

# JWT Configuration
JWT_SECRET_KEY=your-super-secret-jwt-key-32-chars-minimum
JWT_EXPIRY_HOURS=24

# Database Configuration
DATABASE_URL=postgres://user:password@host:port/db
POSTGRES_USER=psychicadmin
POSTGRES_PASSWORD=your_password
POSTGRES_DB=psychicdb
```

## 🔍 Testing Strategy

1. **Unit Tests**: Test individual components
2. **Integration Tests**: Test OAuth flow
3. **JWT Tests**: Test token generation and validation
4. **Security Tests**: Test authentication bypass
5. **Load Tests**: Test JWT validation under load
6. **Browser Tests**: Test complete user flows

## 🎯 Benefits of JWT vs Sessions

### JWT Advantages ✅

- **Stateless**: No server-side session storage
- **Scalable**: Works across multiple servers
- **Mobile Friendly**: Easy to implement in mobile apps
- **Performance**: No database lookups for authentication
- **CORS Friendly**: Works well with cross-origin requests

### Session Disadvantages (Avoided)

- **Stateful**: Requires server-side session storage
- **Scalability Issues**: Session replication across servers
- **Database Dependencies**: Session lookups on every request
- **Mobile Complexity**: Session management in mobile apps
