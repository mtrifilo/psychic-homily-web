# Frontend Integration Guide

This guide explains how to integrate your frontend with the Psychic Homily backend API.

## **🌐 API Endpoints**

- **Development**: `http://localhost:8080`
- **Production**: `https://api.psychichomily.com`

## **🔧 Frontend Configuration**

### **1. Environment Variables**

Create environment files for your frontend:

**.env.development**

```bash
VITE_API_BASE_URL=http://localhost:8080
```

**.env.production**

```bash
VITE_API_BASE_URL=https://api.psychichomily.com
```

### **2. API Client Configuration**

```javascript
// api/client.js
const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL || "http://localhost:8080";

class ApiClient {
  constructor() {
    this.baseURL = API_BASE_URL;
  }

  // Get auth token from localStorage
  getAuthToken() {
    return localStorage.getItem("auth_token");
  }

  // Set auth token in localStorage
  setAuthToken(token) {
    localStorage.setItem("auth_token", token);
  }

  // Remove auth token from localStorage
  removeAuthToken() {
    localStorage.removeItem("auth_token");
  }

  // Make authenticated API request
  async request(endpoint, options = {}) {
    const token = this.getAuthToken();

    const config = {
      ...options,
      headers: {
        "Content-Type": "application/json",
        ...options.headers,
      },
    };

    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }

    const response = await fetch(`${this.baseURL}${endpoint}`, config);

    if (response.status === 401) {
      // Token expired or invalid
      this.removeAuthToken();
      window.location.href = "/login";
      return;
    }

    return response;
  }

  // API methods
  async get(endpoint) {
    return this.request(endpoint, { method: "GET" });
  }

  async post(endpoint, data) {
    return this.request(endpoint, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async delete(endpoint) {
    return this.request(endpoint, { method: "DELETE" });
  }
}

export const apiClient = new ApiClient();
```

## **🔐 Authentication Flow**

### **1. OAuth Login Component**

```javascript
// components/OAuthLogin.jsx
import { apiClient } from "../api/client";

export function OAuthLogin() {
  const handleOAuthLogin = (provider) => {
    // Redirect to backend OAuth endpoint
    window.location.href = `${apiClient.baseURL}/auth/login/${provider}`;
  };

  return (
    <div className="oauth-login">
      <button
        onClick={() => handleOAuthLogin("google")}
        className="btn btn-google"
      >
        Login with Google
      </button>
      {/* Add GitHub when configured */}
      <button
        onClick={() => handleOAuthLogin("github")}
        className="btn btn-github"
        disabled
      >
        Login with GitHub (Coming Soon)
      </button>
    </div>
  );
}
```

### **2. OAuth Flow Process**

1. **User clicks "Login with Google"**

   ```javascript
   window.location.href = "http://localhost:8080/auth/login/google";
   ```

2. **Backend redirects to Google OAuth**

   ```
   Backend → Google OAuth → User authenticates → Google redirects back
   ```

3. **Backend processes callback and returns JWT**

   ```json
   {
     "success": true,
     "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
     "user": {
       "id": 1,
       "email": "matt.trifilo@gmail.com"
     }
   }
   ```

4. **Frontend stores JWT and redirects**
   ```javascript
   // Parse the response and store token
   apiClient.setAuthToken(data.token);
   localStorage.setItem("user", JSON.stringify(data.user));
   window.location.href = "/dashboard";
   ```

### **3. Protected Route Component**

```javascript
// components/ProtectedRoute.jsx
import { useEffect, useState } from "react";
import { apiClient } from "../api/client";

export function ProtectedRoute({ children }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const checkAuth = async () => {
      const token = apiClient.getAuthToken();

      if (!token) {
        window.location.href = "/login";
        return;
      }

      try {
        // Verify token by calling protected endpoint
        const response = await apiClient.get("/auth/profile");
        const data = await response.json();

        if (data.success) {
          setIsAuthenticated(true);
        } else {
          apiClient.removeAuthToken();
          window.location.href = "/login";
        }
      } catch (error) {
        apiClient.removeAuthToken();
        window.location.href = "/login";
      } finally {
        setIsLoading(false);
      }
    };

    checkAuth();
  }, []);

  if (isLoading) {
    return <div>Loading...</div>;
  }

  return isAuthenticated ? children : null;
}
```

## **📱 API Usage Examples**

### **Authentication Endpoints**

```javascript
// Get user profile (protected)
const response = await apiClient.get("/auth/profile");
const profile = await response.json();
console.log(profile);
// Expected: { success: true, user: { id: 1, email: "..." }, message: "Profile retrieved" }

// Refresh JWT token (protected)
const response = await apiClient.post("/auth/refresh");
const data = await response.json();
if (data.success) {
  apiClient.setAuthToken(data.token);
}

// Logout (public)
const response = await apiClient.post("/auth/logout");
apiClient.removeAuthToken();
```

### **Application Endpoints**

```javascript
// Submit a show (public)
const showData = {
  title: "Amazing Band Live",
  date: "2024-01-15",
  venue: "Cool Venue",
  description: "Epic show description",
};
const response = await apiClient.post("/show", showData);
const result = await response.json();

// Health check (public)
const response = await apiClient.get("/health");
const health = await response.json();
console.log(health); // { status: "ok" }
```

## **🔍 Available API Endpoints**

### **Public Endpoints**

- `GET /health` - Health check
- `GET /openapi.json` - API documentation
- `POST /show` - Submit show information
- `POST /auth/logout` - Logout user

### **OAuth Endpoints (Redirects)**

- `GET /auth/login/google` - Initiate Google OAuth
- `GET /auth/callback/google` - Google OAuth callback

### **Protected Endpoints (Require JWT)**

- `GET /auth/profile` - Get user profile
- `POST /auth/refresh` - Refresh JWT token

## **🛠️ Development Setup**

### **1. Start Backend**

```bash
cd backend
go run cmd/server/main.go
# Server runs on http://localhost:8080
```

### **2. Test OAuth Flow**

1. Visit: `http://localhost:8080/auth/login/google`
2. Complete Google OAuth
3. Backend returns JSON with JWT token
4. Store token for authenticated requests

### **3. Test Protected Endpoints**

```bash
# Get JWT token first via OAuth, then:
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  http://localhost:8080/auth/profile
```

## **🚀 Production Deployment**

### **1. Backend Configuration**

- Update `OAUTH_REDIRECT_URL` to production domain
- Configure Google OAuth with production callback URLs
- Set secure `JWT_SECRET_KEY` and `OAUTH_SECRET_KEY`

### **2. Frontend Configuration**

- Set `VITE_API_BASE_URL` to production API URL
- Configure CORS on backend for frontend domain
- Update OAuth redirect handling for production

### **3. CORS Configuration**

Backend is configured for these origins:

- `http://localhost:3000` (React dev)
- `http://localhost:5173` (Vite dev)
- `https://psychichomily.com` (Production)
- `https://www.psychichomily.com` (Production)

## **🔍 API Documentation**

Visit `http://localhost:8080/openapi.json` to view the complete OpenAPI specification with all available endpoints, request/response schemas, and examples.

## **🛠️ Troubleshooting**

### **OAuth Issues**

- Ensure Google OAuth redirect URI matches: `http://localhost:8080/auth/callback/google`
- Check that `OAUTH_SECRET_KEY` is set and consistent
- Verify session cookies are working (SameSite=Lax for localhost)

### **JWT Issues**

- Verify `JWT_SECRET_KEY` is set and doesn't change between restarts
- Check token expiration (default: 24 hours)
- Ensure `Authorization: Bearer <token>` header format

### **CORS Issues**

- Check that frontend origin is in CORS allowed origins list
- Verify preflight requests are handled properly
- Ensure credentials are included in requests if needed
